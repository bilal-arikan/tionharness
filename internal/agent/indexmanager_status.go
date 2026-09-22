package agent

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// IndexStatus is one line of the agent-facing status report: what TionHarness
// knows about the index covering a root, for one tool.
//
// It is a flattened view rather than the raw ledger Entry because the two
// managed tools do not share a backing store. zvec-grep entries come straight
// from the ledger; codebase-memory has no ledger integration yet, so its rows
// are assembled from the in-process auto-index guards and say so via Managed.
type IndexStatus struct {
	Tool        string `json:"tool"`
	Root        string `json:"root"`
	Phase       string `json:"phase"`
	Action      string `json:"action,omitempty"`
	Embedding   string `json:"embedding,omitempty"`
	ToolVersion string `json:"toolVersion,omitempty"`
	Error       string `json:"error,omitempty"`
	Usable      bool   `json:"usable"`
	// Managed reports whether this tool's index lifecycle is driven through the
	// ledger, and therefore whether refresh/rebuild will do anything for it.
	// False for codebase-memory, whose row is observational only.
	Managed bool `json:"managed"`
	// Note carries the human-readable qualification for an unmanaged or
	// otherwise special row, e.g. why codebase-memory cannot be refreshed here.
	Note string `json:"note,omitempty"`
}

// SearchIndexStatus reports the lifecycle state of every index covering root,
// across the managed tools.
//
// root is the caller's working root. A ledger entry is keyed by the directory
// that actually holds the store, which may be an ANCESTOR of root (a session
// started in a subdirectory is covered by the repository-level index), so the
// lookup resolves the covering root the same way the indexer does instead of
// demanding an exact key match — otherwise a perfectly healthy index reads back
// as "no record" to any agent working in a subdirectory.
func (r *Runtime) SearchIndexStatus(ctx context.Context, root string) []IndexStatus {
	out := make([]IndexStatus, 0, 2)
	if z, ok := r.zvecGrepStatus(root); ok {
		out = append(out, z)
	}
	if c, ok := r.codebaseMemoryStatus(ctx, root); ok {
		out = append(out, c)
	}
	return out
}

// zvecGrepStatus reads the ledger for the index covering root, falling back to a
// disk observation when this process has no record of it yet. The distinction
// matters: "never touched in this process" and "looked and found nothing" are
// different answers, and only a disk read may assert the second.
func (r *Runtime) zvecGrepStatus(root string) (IndexStatus, bool) {
	if !r.ZvecGrepEnabled() {
		return IndexStatus{}, false
	}
	root = strings.TrimSpace(root)
	if root == "" || !isAbsPath(root) {
		return IndexStatus{}, false
	}

	target := zvecGrepIndexedRoot(root)
	if target == "" {
		target = zvecGrepIndexTarget(root)
	}

	st := IndexStatus{Tool: exttoolsZvecGrepName, Root: target, Managed: true}
	if e, seen := indexLedger.Get(exttoolsZvecGrepName, target); seen {
		st.Phase = string(e.Phase)
		st.Action, st.Embedding, st.ToolVersion = e.Action, e.Embedding, e.ToolVersion
		st.Error, st.Usable = e.Error, e.Usable()
		return st, true
	}

	// No ledger record: report what is on disk right now, without recording it.
	// A status read must not mutate the ledger — an Observe here would overwrite
	// a phase a concurrent run is about to set.
	obs := r.observeZvecGrep(target)
	phase := obsPhase(obs)
	st.Phase = string(phase)
	st.Embedding = obs.Embedding
	st.Usable = indexstate.Entry{Phase: phase}.Usable()
	if !obs.Exists {
		st.Note = "bu kök için indeks yok; search_index refresh ile oluşturulabilir"
	}
	return st, true
}

// codebaseMemoryStatus reports what is CHEAPLY knowable about the code graph's
// coverage of root — no CLI call, no store read.
//
// codebase-memory is not in the ledger (a separate card). What this process does
// know is whether it has already fired its best-effort auto-index for this
// directory in this run, and whether one is in flight, which is exactly the
// question an agent asks before deciding to grep instead. Anything beyond that
// (does the store really hold this project, how fresh is it) would cost a
// subprocess per status call, so it is reported as unknown rather than guessed.
func (r *Runtime) codebaseMemoryStatus(ctx context.Context, root string) (IndexStatus, bool) {
	if !r.CodebaseMemoryEnabled() || r.codebaseMemoryCmd(ctx) == "" {
		return IndexStatus{}, false
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return IndexStatus{}, false
	}

	st := IndexStatus{
		Tool:    codebaseMemoryToolName,
		Root:    root,
		Managed: false,
		Note: "kod grafiği indeksi TionHarness kayıt defterinde tutulmuyor; durum yalnızca bu " +
			"süreçteki otomatik indeksleme durumundan okunur. Tazeleme için oturumun normal " +
			"akışındaki otomatik indeksleme yeterlidir; index_repository'yi kendin çağırma.",
	}
	switch {
	case isEphemeralWorkdir(root):
		st.Phase = string(indexstate.PhaseMissing)
		st.Note = "geçici çalışma kopyası hiç indekslenmez; üst deponun indeksi bu kodu zaten kapsar"
	case r.codebaseIndexInFlight(root):
		st.Phase = string(indexstate.PhaseIndexing)
	default:
		if _, fired := r.cbmIndexed.Load(root); fired {
			// The auto-index ran for this directory in this process and did not
			// clear its guard, i.e. it did not fail. That is evidence of coverage,
			// not proof the store answers a given query, so it reads as usable
			// rather than ready.
			st.Phase = string(indexstate.PhaseStale)
			st.Usable = true
			st.Note = "bu süreçte otomatik indekslendi; tazeliği sunucunun kendi izleyicisi yönetir"
		} else {
			st.Phase = "unknown"
			st.Note = "bu süreçte indekslenmedi; sunucunun kendi deposunda kayıt olabilir. " + st.Note
		}
	}
	return st, true
}

// codebaseIndexInFlight reports whether the process-wide codebase-memory
// auto-index is running for root. Mirrors the key EnsureCodebaseIndexed builds
// so the two cannot disagree about what "running" means.
func (r *Runtime) codebaseIndexInFlight(root string) bool {
	key := filepath.ToSlash(root)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	_, running := codebaseIndexRunning.Load(key)
	return running
}
