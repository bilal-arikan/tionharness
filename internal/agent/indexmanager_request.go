package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// ErrIndexRootNotAllowed is returned when a caller asks TionHarness to index a
// root it has no business indexing: a home directory, a volume root, an
// ephemeral worktree/scratchpad, or a relative path.
//
// The agent-facing tool surfaces this verbatim. It is a separate error from
// ErrUnknownIndexTool because the two mean different things to the caller: an
// unknown tool is a typo, a refused root is a policy decision it must not
// retry with a different spelling of the same path.
var ErrIndexRootNotAllowed = fmt.Errorf("bu kök için indeks yönetimi yapılmıyor")

// ErrIndexRunInFlight is returned when a refresh/rebuild is requested for a
// root that already has a run in flight. It is deliberately NOT an error the
// caller should treat as failure — the work it asked for is already happening —
// so the tool reports it as an informational outcome rather than a fault.
var ErrIndexRunInFlight = fmt.Errorf("bu kök için zaten bir indeks çalışması sürüyor")

// IndexRequest names one explicit lifecycle request: refresh (re-embed in
// place, keeping the store) or rebuild (discard the store and build it again).
//
// Create is deliberately absent. A root with no index is brought up by
// EnsureZvecGrepIndex on the normal turn path, and a caller asking for a
// refresh of a missing index means "make it usable" — which RequestIndexRun
// resolves to a create rather than refusing on a technicality.
type IndexRequest struct {
	Tool   string
	Root   string
	Action string
}

// RequestIndexRun performs an EXPLICIT refresh or rebuild of one managed index,
// on behalf of a caller (the search_index tool, or a user action) rather than
// the automatic Decide path.
//
// It is the forced counterpart of EnsureZvecGrepIndex: where that one asks
// indexstate.Decide whether anything needs doing and does nothing when the
// index is already ready, this one runs what was asked for. That is the entire
// point of an explicit request — an agent that has just seen [INDEX_MISSING]
// against an index the ledger believes is ready needs a way to say "build it
// anyway" without waiting a day for the staleness threshold.
//
// Every guard EnsureZvecGrepIndex applies still applies here, because each one
// prevents a concrete harm rather than merely saving work: the root must be
// absolute, must not be a throwaway working copy, must not be a home or volume
// root, must be excludable from git (checked inside runZvecGrepAction), and
// only one run per (tool, root) may be in flight at a time.
//
// The run is asynchronous, like every other index run: the returned entry is
// the CLAIMED entry (PhaseIndexing), not a finished one.
func (r *Runtime) RequestIndexRun(ctx context.Context, req IndexRequest) (indexstate.Entry, error) {
	switch req.Tool {
	case exttoolsZvecGrepName:
		return r.requestZvecGrepRun(ctx, req.Root, req.Action)
	default:
		return indexstate.Entry{}, fmt.Errorf("%w: %s", ErrUnknownIndexTool, req.Tool)
	}
}

// requestZvecGrepRun is RequestIndexRun's zvec-grep arm.
func (r *Runtime) requestZvecGrepRun(ctx context.Context, root, action string) (indexstate.Entry, error) {
	switch action {
	case indexstate.ActionRefresh, indexstate.ActionRebuild:
	default:
		return indexstate.Entry{}, fmt.Errorf("desteklenmeyen indeks eylemi: %s", action)
	}

	root = strings.TrimSpace(root)
	if root == "" || !isAbsPath(root) {
		return indexstate.Entry{}, fmt.Errorf("%w: mutlak bir yol gerekli", ErrIndexRootNotAllowed)
	}
	if isEphemeralWorkdir(root) {
		return indexstate.Entry{}, fmt.Errorf("%w: geçici çalışma kopyası (worktree/scratchpad)", ErrIndexRootNotAllowed)
	}
	if zvecGrepBroadDir(root) {
		return indexstate.Entry{}, fmt.Errorf("%w: ev dizini veya disk kökü", ErrIndexRootNotAllowed)
	}

	server, ok := r.zvecGrepServer(ctx)
	if !ok {
		return indexstate.Entry{}, fmt.Errorf("zvec-grep sunucusu etkin değil")
	}
	command := zvecGrepCLI(server)
	if command == "" {
		return indexstate.Entry{}, fmt.Errorf("zg çalıştırılabiliri bulunamadı")
	}

	// An index at or above root covers root; that root, not the requested
	// directory, is the entry — the same rule the automatic path follows, so an
	// explicit refresh never mints a nested index beside the real one.
	target := zvecGrepIndexedRoot(root)
	if target == "" {
		target = zvecGrepIndexTarget(root)
	}

	want := indexstate.Desired{
		Embedding:   zvecGrepEmbedding(),
		ToolVersion: zvecGrepToolVersion(ctx, command),
	}

	// A refresh of an index that is not there yet is a create: the store has to
	// exist before anything can be re-embedded into it. Resolving it here keeps
	// the ledger's Action honest about what actually ran.
	effective := action
	if action == indexstate.ActionRefresh && !r.observeZvecGrep(target).Exists {
		effective = indexstate.ActionCreate
	}

	entry, claimed := indexLedger.Begin(exttoolsZvecGrepName, target, effective)
	if !claimed {
		return entry, ErrIndexRunInFlight
	}

	r.logger.Info("search index run requested", "tool", exttoolsZvecGrepName,
		"root", target, "action", effective)
	go r.runZvecGrepAction(command, target, effective, entry.Run, want)
	return entry, nil
}
