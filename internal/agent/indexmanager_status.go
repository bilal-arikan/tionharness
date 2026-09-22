package agent

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// IndexStatus is one line of the agent-facing status report: what TionHarness
// knows about the index covering a root, for one tool.
//
// It is a flattened view rather than the raw ledger Entry so a row can carry a
// Note, and so a root the ledger has no record of yet can still be answered
// from a direct observation of the tool's store.
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
	// True for every tool the manager knows (zvec-grep and codebase-memory).
	Managed bool `json:"managed"`
	// Note carries the human-readable qualification for a special row, e.g. an
	// ephemeral working copy that is never indexed.
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

// codebaseMemoryStatus reports the code graph's coverage of root from the
// ledger, falling back to asking the server when this process has no record.
//
// The fallback costs one short CLI call, which is acceptable for an explicit
// status request (the automatic per-turn path records its observation in the
// ledger, so a working session normally never reaches it). Like zvec-grep's
// fallback it does NOT record what it saw: a status read must not overwrite a
// phase a concurrent run is about to set. A server answer that cannot be read
// reports phase "unknown" with the reason — never a guessed ready.
func (r *Runtime) codebaseMemoryStatus(ctx context.Context, root string) (IndexStatus, bool) {
	command := r.codebaseMemoryCmd(ctx)
	if command == "" {
		return IndexStatus{}, false
	}
	root = strings.TrimSpace(root)
	if root == "" || !isAbsPath(root) {
		return IndexStatus{}, false
	}

	st := IndexStatus{Tool: codebaseMemoryToolName, Root: root, Managed: true}
	if isEphemeralWorkdir(root) {
		st.Phase = string(indexstate.PhaseMissing)
		st.Note = "geçici çalışma kopyası hiç indekslenmez; üst deponun indeksi bu kodu zaten kapsar"
		return st, true
	}
	if e, seen := indexLedger.Get(codebaseMemoryToolName, root); seen {
		st.Phase = string(e.Phase)
		st.Action, st.Error, st.Usable = e.Action, e.Error, e.Usable()
		return st, true
	}

	obs, err := observeCodebaseMemory(ctx, command, root)
	if err != nil {
		st.Phase = "unknown"
		st.Note = "kod grafiği deposu okunamadı: " + err.Error()
		return st, true
	}
	phase := obsPhase(obs)
	st.Phase = string(phase)
	st.Usable = indexstate.Entry{Phase: phase}.Usable()
	if !obs.Exists {
		st.Note = "bu kök için kod grafiği yok; search_index refresh ile oluşturulabilir"
	}
	return st, true
}
