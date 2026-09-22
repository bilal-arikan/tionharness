package agent

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// indexLedger is the process-wide (tool, root) lifecycle ledger. Process-wide,
// not per-Runtime, for the same reason the old run guards were: two workspace
// runtimes opened on the same repository share one daemon and one store on disk,
// so a second runtime must see the first one's run rather than start its own.
var indexLedger = indexstate.New()

// IndexLedger exposes the ledger for readers outside this package — the HTTP
// status endpoint. Read-only by convention: the transitions live here, next to
// the code that actually runs the indexer.
func IndexLedger() *indexstate.Manager { return indexLedger }

// zvecGrepStaleAfter is how long an index may go without an update before the
// manager calls it stale and schedules a refresh.
//
// zvec-grep's daemon watches the tree and re-embeds on change, so a fresh
// manifest is the normal case and this threshold only catches an index whose
// watcher was not running — TionHarness closed, the machine asleep, the daemon
// crashed. A day is long enough that a working watcher never trips it.
const zvecGrepStaleAfter = 24 * time.Hour

// zvecGrepToolVersion reads the installed `zg` version for the rebuild check,
// behind a var so tests can stub it. Errors are folded into "" — an unreadable
// version is UNKNOWN, and indexstate.Decide deliberately does not rebuild on an
// unknown, so a probe failure cannot trigger a repository-wide re-embed.
var zvecGrepToolVersion = func(ctx context.Context, command string) string {
	v, err := exttoolsLocalVersion(ctx, command, []string{"--version"})
	if err != nil {
		return ""
	}
	return v
}

// EnsureZvecGrepIndex brings the index covering cwd to a usable state and
// records every transition in the ledger.
//
// This is the managed replacement for the old one-shot create. It still only
// ever runs in the background and still reports whether a usable index exists or
// is on its way, but it now also notices an index built with a DIFFERENT
// embedding model or by a different tool version (rebuild), and one whose
// watcher has not touched it in a day (refresh).
//
// Every skip that guarded the old path is preserved, because each one prevents a
// concrete harm rather than merely saving work:
//   - a relative cwd cannot be resolved to a root at all;
//   - an ephemeral worktree or scratchpad is a throwaway copy of a repository the
//     parent index already covers;
//   - a home directory or volume root would embed far more than a project into a
//     store written at its top;
//   - a root whose index cannot be excluded from git is left UNINDEXED, because
//     the alternative is an untracked binary store one `git add -A` from a commit.
func (r *Runtime) EnsureZvecGrepIndex(ctx context.Context, cwd string) bool {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" || !isAbsPath(cwd) {
		return false
	}
	if isEphemeralWorkdir(cwd) {
		r.logger.Debug("zvec-grep index skipped: ephemeral working copy", "cwd", cwd)
		return false
	}
	if zvecGrepBroadDir(cwd) {
		r.logger.Debug("zvec-grep index skipped: home or volume root", "cwd", cwd)
		return false
	}
	server, ok := r.zvecGrepServer(ctx)
	if !ok {
		return false
	}
	command := zvecGrepCLI(server)
	if command == "" {
		r.logger.Warn("zvec-grep index skipped: zg executable not found", "server", server.Name, "cwd", cwd)
		return false
	}

	// An index at or above cwd covers cwd; that root, not cwd, is the entry.
	root := zvecGrepIndexedRoot(cwd)
	if root == "" {
		root = zvecGrepIndexTarget(cwd)
	}

	want := indexstate.Desired{
		Embedding:   zvecGrepEmbedding(),
		ToolVersion: zvecGrepToolVersion(ctx, command),
	}
	obs := r.observeZvecGrep(root)
	// A run this process already completed is authoritative over a disk reading:
	// the ledger records what was built, and re-deriving "missing" from the
	// filesystem would start the same create again on the next turn.
	if prev, seen := indexLedger.Get(exttoolsZvecGrepName, root); seen && prev.Phase == indexstate.PhaseReady {
		obs.Exists = true
		// Each field falls back independently: the manifest names the embedding
		// but records no tool version, so the ledger is the ONLY source for the
		// version the store was built by — merging them together would leave the
		// version permanently unknown and the version-change rebuild unreachable.
		if obs.Embedding == "" {
			obs.Embedding = prev.Embedding
		}
		if obs.ToolVersion == "" {
			obs.ToolVersion = prev.ToolVersion
		}
	}
	indexLedger.Observe(exttoolsZvecGrepName, root, obsPhase(obs), obs.Embedding, obs.ToolVersion)

	action, _ := indexstate.Decide(obs, want)
	if action == "" {
		return true // ready, nothing to do
	}

	entry, claimed := indexLedger.Begin(exttoolsZvecGrepName, root, action)
	if !claimed {
		r.logger.Info("zvec-grep index already running, skipping", "root", root, "action", entry.Action)
		return true // a run claimed by another caller is still a run on its way
	}

	go r.runZvecGrepAction(command, root, action, entry.Run, want)

	// True covers both "an index is here" and "one is on its way": the caller
	// uses this to decide whether to tell the model an index is coming, and a
	// scheduled run is exactly that.
	return true
}

// runZvecGrepAction performs one claimed lifecycle run and closes the ledger
// entry. It always closes it: a run that returns without Succeed or Fail would
// strand the entry in PhaseIndexing and block every later attempt.
func (r *Runtime) runZvecGrepAction(command, root, action string, run uint64, want indexstate.Desired) {
	// The exclude entry must exist BEFORE the store does, so the index never
	// appears as untracked files even briefly. A root that cannot be excluded is
	// not indexed at all.
	if err := ensureZvecGrepGitExclude(root); err != nil {
		r.logger.Warn("zvec-grep index skipped: could not exclude the index from git",
			"root", root, "action", action, "error", err)
		r.failIndexRun(exttoolsZvecGrepName, root, run, "indeks git'ten dışlanamadı: "+err.Error())
		return
	}

	// A rebuild discards the old store first: `zg index` over a store built with
	// a different embedding model would add incomparable vectors to it rather
	// than replace them.
	if action == indexstate.ActionRebuild {
		if err := os.RemoveAll(zvecGrepIndexPath(root)); err != nil {
			r.logger.Warn("zvec-grep rebuild failed: could not remove the old index",
				"root", root, "error", err)
			r.failIndexRun(exttoolsZvecGrepName, root, run, "eski indeks silinemedi: "+err.Error())
			return
		}
	}

	ictx, cancel := context.WithTimeout(context.Background(), zvecGrepIndexTimeout)
	defer cancel()

	r.logger.Info("zvec-grep index run started", "root", root, "action", action, "embedding", want.Embedding)
	out, err := runZvecGrepIndex(ictx, command, root, want.Embedding)
	if err != nil {
		reason := err.Error()
		if tail := zvecGrepTail(out); tail != "" {
			reason += ": " + tail
		}
		// Logged AND recorded: a failed index must never read back as ready.
		r.logger.Warn("zvec-grep index run failed", "root", root, "action", action, "error", err,
			"output", zvecGrepTail(out))
		r.failIndexRun(exttoolsZvecGrepName, root, run, reason)
		return
	}

	// Record what the store was ACTUALLY built with, read back from the manifest
	// the run just wrote, rather than what was requested — if zvec-grep fell back
	// to a different model, the ledger must show that model, or the next start
	// would compare against the wrong value and never rebuild.
	built := want
	if info, mErr := readZvecGrepManifest(root); mErr == nil && info.Embedding != "" {
		built.Embedding = info.Embedding
	}
	if _, err := indexLedger.Succeed(exttoolsZvecGrepName, root, run, built.Embedding, built.ToolVersion); err != nil {
		// The store on disk was built, but this run no longer owns the entry (it
		// was Forgotten or re-claimed meanwhile). The ledger keeps what the owning
		// run records; this outcome is reported, not silently dropped.
		r.logger.Error("zvec-grep index run finished but the ledger rejected it",
			"root", root, "action", action, "run", run, "error", err)
		return
	}
	r.logger.Info("zvec-grep index run finished", "root", root, "action", action, "embedding", built.Embedding)
}

// failIndexRun records a failed run in the ledger. A rejected close (the run no
// longer holds its claim) is logged at error level: the failure reason would
// otherwise vanish without a trace.
func (r *Runtime) failIndexRun(tool, root string, run uint64, reason string) {
	if _, err := indexLedger.Fail(tool, root, run, reason); err != nil {
		r.logger.Error("index run failed but the ledger rejected the failure",
			"tool", tool, "root", root, "run", run, "reason", reason, "error", err)
	}
}

// observeZvecGrep inspects the store at root and reports what is there.
//
// A manifest that exists but cannot be read is reported as Exists with an EMPTY
// embedding, which is "unknown" to Decide: an unreadable manifest is not
// evidence that the model changed, so it does not trigger a rebuild. It is
// logged so the condition is visible instead of silent.
func (r *Runtime) observeZvecGrep(root string) indexstate.Observed {
	st, err := os.Stat(zvecGrepManifestPath(root))
	if err != nil || st.IsDir() {
		return indexstate.Observed{Exists: false}
	}
	obs := indexstate.Observed{Exists: true}
	info, err := readZvecGrepManifest(root)
	if err != nil {
		r.logger.Warn("zvec-grep index manifest unreadable", "root", root, "error", err)
		return obs
	}
	obs.Embedding = info.Embedding
	if !info.UpdatedAt.IsZero() {
		obs.Stale = time.Since(info.UpdatedAt) > zvecGrepStaleAfter
	}
	return obs
}

// obsPhase converts a disk observation into the phase to record before any run
// starts.
func obsPhase(obs indexstate.Observed) indexstate.Phase {
	switch {
	case !obs.Exists:
		return indexstate.PhaseMissing
	case obs.Stale:
		return indexstate.PhaseStale
	default:
		return indexstate.PhaseReady
	}
}

// DropZvecGrepIndex deletes the index at root after the user confirmed it.
//
// confirmRoot is the user's confirmation and must repeat root exactly. Nothing
// in TionHarness calls this automatically — a vanished project directory is
// Forgotten from the ledger instead, since re-indexing a large repository is
// expensive and a wrong delete cannot be undone.
func (r *Runtime) DropZvecGrepIndex(root, confirmRoot string) error {
	err := indexLedger.Drop(indexstate.DropRequest{
		Tool:        exttoolsZvecGrepName,
		Root:        root,
		ConfirmRoot: confirmRoot,
		IndexDir:    zvecGrepIndexDir,
	})
	if err != nil {
		if !errors.Is(err, indexstate.ErrDropNotConfirmed) {
			r.logger.Warn("zvec-grep index drop failed", "root", root, "error", err)
		}
		return err
	}
	r.logger.Info("zvec-grep index dropped", "root", root)
	return nil
}
