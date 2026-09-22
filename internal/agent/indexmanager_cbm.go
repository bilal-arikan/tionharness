package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/indexstate"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/proc"
	"github.com/bilal-arikan/tionharness/internal/procwatch"
)

// codebaseMemoryIndexTimeout bounds one index_repository run. An incremental
// run is seconds; a first full index of a large repository is minutes. The bound
// exists so a wedged child cannot hold the ledger's per-root claim for the life
// of the process.
const codebaseMemoryIndexTimeout = 30 * time.Minute

// codebaseMemoryProbeTimeout bounds the read-only CLI calls (index_status,
// delete_project). The server opens its store on every CLI call, which is a
// couple of seconds on a large cache — never minutes.
const codebaseMemoryProbeTimeout = 60 * time.Second

// codebaseMemoryCLI runs one `codebase-memory-mcp cli <tool> [flags]` call and
// returns the output that carries its JSON reply. The server answers a success
// on stdout but writes an error reply ({"error":...} / {"status":"not_found"},
// exit status 1) to stderr, interleaved with its `level=` log lines — so stdout
// is returned when it holds anything, stderr otherwise, and the parser picks the
// JSON line out of it. Behind a var so tests can stub the binary.
var codebaseMemoryCLI = func(ctx context.Context, command string, args ...string) ([]byte, error) {
	cmd := proc.CommandContext(ctx, command, append([]string{"cli"}, args...)...)
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	h := procwatch.Default().Begin(ctx, procwatch.Meta{
		Kind: procwatch.KindExternal, Label: "codebase-memory",
		Command: command + " cli " + strings.Join(args, " "),
	})
	out, err := cmd.Output()
	h.Started(cmd)
	h.AppendOutput(stderr.String())
	h.Finish(err)
	if len(bytes.TrimSpace(out)) == 0 {
		return stderr.Bytes(), err
	}
	return out, err
}

// codebaseMemoryReply is the subset of the CLI's JSON reply the manager reads.
// index_status answers {"status":"ready",...} for an indexed project and either
// {"status":"not_found"} or {"error":"project not found or not indexed"} for one
// the store does not hold (both with exit status 1).
type codebaseMemoryReply struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// parseCodebaseMemoryReply decodes the CLI's reply: the last line of out that
// is a JSON object. Log lines around it are ignored.
func parseCodebaseMemoryReply(out []byte) (codebaseMemoryReply, error) {
	var rep codebaseMemoryReply
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range slices.Backward(lines) {
		line := strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			return rep, fmt.Errorf("codebase-memory yanıtı çözülemedi: %w", err)
		}
		return rep, nil
	}
	return rep, fmt.Errorf("codebase-memory yanıtı JSON değil: %q", zvecGrepTail(out))
}

// codebaseMemoryProjectID is the server's project id for root — the key its
// store uses and the argument its CLI tools take.
func codebaseMemoryProjectID(root string) string {
	return mcp.ProjectIDForPath(filepath.ToSlash(root))
}

// observeCodebaseMemory asks the server whether its store holds root.
//
// codebase-memory keeps its store in the user's cache directory under its own
// naming, so unlike zvec-grep there is no manifest to stat: the server itself is
// the only authority. Its file watcher keeps an indexed project current, so the
// server reports no staleness signal and none is invented here — an indexed
// project reads as ready.
//
// An answer the manager does not recognise is an ERROR, not a guess: a status
// read must never advertise an index as usable on a reply it could not parse.
func observeCodebaseMemory(ctx context.Context, command, root string) (indexstate.Observed, error) {
	pctx, cancel := context.WithTimeout(ctx, codebaseMemoryProbeTimeout)
	defer cancel()
	out, runErr := codebaseMemoryCLI(pctx, command, "index_status", "--project", codebaseMemoryProjectID(root))
	rep, err := parseCodebaseMemoryReply(out)
	if err != nil {
		if runErr != nil {
			return indexstate.Observed{}, fmt.Errorf("index_status: %w", runErr)
		}
		return indexstate.Observed{}, err
	}
	switch {
	case rep.Status == "ready":
		return indexstate.Observed{Exists: true}, nil
	case rep.Status == "not_found", strings.Contains(strings.ToLower(rep.Error), "not found"):
		return indexstate.Observed{Exists: false}, nil
	case rep.Error != "":
		return indexstate.Observed{}, fmt.Errorf("index_status: %s", rep.Error)
	default:
		return indexstate.Observed{}, fmt.Errorf("index_status: tanınmayan durum %q", rep.Status)
	}
}

// codebaseMemoryRootAllowed applies the root guards every codebase-memory run
// obeys, on both the automatic and the explicit path. Each one prevents a
// concrete harm: a relative path cannot be resolved to a project id, a
// worktree/scratchpad is a throwaway copy the parent index already covers (the
// 16GB cache incident — see ephemeralIndexDirs), and a home directory or volume
// root would pull far more than a project into one graph.
func codebaseMemoryRootAllowed(root string) error {
	switch {
	case root == "" || !isAbsPath(root):
		return fmt.Errorf("%w: mutlak bir yol gerekli", ErrIndexRootNotAllowed)
	case isEphemeralWorkdir(root):
		return fmt.Errorf("%w: geçici çalışma kopyası (worktree/scratchpad)", ErrIndexRootNotAllowed)
	case zvecGrepBroadDir(root):
		return fmt.Errorf("%w: ev dizini veya disk kökü", ErrIndexRootNotAllowed)
	}
	return nil
}

// codebaseMemoryAction picks the action to record for a run whose target was
// just observed: a store that does not hold the project is a create, one that
// does is a refresh. An observation error leaves the choice to fallback — the
// run is the same `index_repository` either way, only the label differs.
func codebaseMemoryAction(obs indexstate.Observed, obsErr error, fallback string) string {
	if obsErr != nil {
		return fallback
	}
	if !obs.Exists {
		return indexstate.ActionCreate
	}
	return indexstate.ActionRefresh
}

// runCodebaseMemoryAction performs one claimed lifecycle run and closes the
// ledger entry. It always closes it: a run that returns without Succeed or Fail
// would strand the entry in PhaseIndexing and block every later attempt.
//
// A rebuild deletes the project from the server's store first, so the graph is
// re-extracted from scratch rather than incrementally patched.
func (r *Runtime) runCodebaseMemoryAction(command, root, action string, run uint64) bool {
	ctx, cancel := context.WithTimeout(context.Background(), codebaseMemoryIndexTimeout)
	defer cancel()

	if action == indexstate.ActionRebuild {
		if err := deleteCodebaseMemoryProject(ctx, command, root); err != nil {
			r.logger.Warn("codebase-memory rebuild failed: could not delete the old project",
				"root", root, "error", err)
			r.failIndexRun(codebaseMemoryToolName, root, run, "eski proje silinemedi: "+err.Error())
			return false
		}
	}

	r.logger.Info("codebase-memory index run started", "root", root, "action", action)
	// Flag form: codebase-memory-mcp 0.10 deprecated raw-JSON CLI args.
	out, err := runIndexRepository(ctx, command, filepath.ToSlash(root))
	if err != nil {
		reason := err.Error()
		if tail := zvecGrepTail(out); tail != "" {
			reason += ": " + tail
		}
		// Logged AND recorded: a failed index must never read back as ready.
		r.logger.Warn("codebase-memory index run failed", "root", root, "action", action,
			"error", err, "output", strings.TrimSpace(string(out)))
		r.failIndexRun(codebaseMemoryToolName, root, run, reason)
		return false
	}
	if _, err := indexLedger.Succeed(codebaseMemoryToolName, root, run, "", ""); err != nil {
		r.logger.Error("codebase-memory index run finished but the ledger rejected it",
			"root", root, "action", action, "run", run, "error", err)
		return false
	}
	r.logger.Info("codebase-memory index run finished", "root", root, "action", action)
	return true
}

// deleteCodebaseMemoryProject removes root's project from the server's store
// through the server's own delete_project tool — the store lives in the user's
// cache directory under the server's naming, so deleting files behind its back
// is not an option. A project the store does not hold counts as deleted.
func deleteCodebaseMemoryProject(ctx context.Context, command, root string) error {
	pctx, cancel := context.WithTimeout(ctx, codebaseMemoryProbeTimeout)
	defer cancel()
	out, runErr := codebaseMemoryCLI(pctx, command, "delete_project", "--project", codebaseMemoryProjectID(root))
	rep, err := parseCodebaseMemoryReply(out)
	if err != nil {
		if runErr != nil {
			return fmt.Errorf("delete_project: %w", runErr)
		}
		return err
	}
	switch {
	case rep.Status == "not_found":
		return nil
	case rep.Error != "":
		return fmt.Errorf("delete_project: %s", rep.Error)
	case runErr != nil:
		return fmt.Errorf("delete_project: %w (durum %q)", runErr, rep.Status)
	}
	return nil
}

// requestCodebaseMemoryRun is RequestIndexRun's codebase-memory arm: an explicit
// refresh (incremental re-index) or rebuild (delete the project, index again).
func (r *Runtime) requestCodebaseMemoryRun(ctx context.Context, root, action string) (indexstate.Entry, error) {
	switch action {
	case indexstate.ActionRefresh, indexstate.ActionRebuild:
	default:
		return indexstate.Entry{}, fmt.Errorf("desteklenmeyen indeks eylemi: %s", action)
	}
	root = strings.TrimSpace(root)
	if err := codebaseMemoryRootAllowed(root); err != nil {
		return indexstate.Entry{}, err
	}
	command := r.codebaseMemoryCmd(ctx)
	if command == "" {
		return indexstate.Entry{}, fmt.Errorf("codebase-memory sunucusu etkin değil")
	}

	// A refresh of a project the store does not hold is a create; resolving it
	// keeps the ledger's Action honest about what actually ran.
	effective := action
	if action == indexstate.ActionRefresh {
		obs, obsErr := observeCodebaseMemory(ctx, command, root)
		if obsErr != nil {
			r.logger.Warn("codebase-memory index observation failed", "root", root, "error", obsErr)
		}
		effective = codebaseMemoryAction(obs, obsErr, action)
	}

	entry, claimed := indexLedger.Begin(codebaseMemoryToolName, root, effective)
	if !claimed {
		return entry, ErrIndexRunInFlight
	}
	// An explicit run covers this root for the rest of the process; the automatic
	// path must not start a second one on the next turn.
	r.cbmIndexed.Store(root, true)
	r.logger.Info("search index run requested", "tool", codebaseMemoryToolName, "root", root, "action", effective)
	go r.runCodebaseMemoryAction(command, root, effective, entry.Run)
	return entry, nil
}

// DropCodebaseMemoryIndex deletes root's project from the server's store after
// the user confirmed it. Same gate as every drop: confirmRoot must repeat root.
// Nothing in TionHarness calls this automatically.
func (r *Runtime) DropCodebaseMemoryIndex(ctx context.Context, root, confirmRoot string) error {
	if err := indexstate.CheckDropConfirmation(codebaseMemoryToolName, root, confirmRoot); err != nil {
		return err
	}
	command := r.codebaseMemoryCmd(ctx)
	if command == "" {
		return fmt.Errorf("codebase-memory sunucusu etkin değil")
	}
	if e, seen := indexLedger.Get(codebaseMemoryToolName, root); seen && e.Phase == indexstate.PhaseIndexing {
		// Deleting the project under a live index_repository would leave the run
		// writing into a store that no longer has it.
		return ErrIndexRunInFlight
	}
	if err := deleteCodebaseMemoryProject(ctx, command, root); err != nil {
		r.logger.Warn("codebase-memory index drop failed", "root", root, "error", err)
		return fmt.Errorf("indeks silinemedi: %w", err)
	}
	indexLedger.Forget(codebaseMemoryToolName, root)
	r.logger.Info("codebase-memory index dropped", "root", root)
	return nil
}
