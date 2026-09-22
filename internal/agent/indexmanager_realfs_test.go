package agent

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/exttools"
	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// realZvecGrepRunTimeout bounds one real `zg index` of the tiny test repository.
// A local model that is not cached yet is prepared on the first run, so this is
// generous; a run that exceeds it is a failure, not a skip.
const realZvecGrepRunTimeout = 3 * time.Minute

// newRealZvecGrepRuntime returns a runtime wired to the INSTALLED zg binary with
// nothing stubbed but the ledger (reset, since it is process-wide). It skips when
// the binary or git is missing: without them the test cannot prove anything, and
// a PASS that ran nothing would be a fake one.
func newRealZvecGrepRuntime(t *testing.T) (*Runtime, *atomic.Int32) {
	t.Helper()
	if testing.Short() {
		t.Skip("real zg index run skipped in -short mode")
	}
	found, zg := exttools.Detect(exttools.ZvecGrepToolName)
	if !found {
		t.Skip("zg (zvec-grep) not installed; real-filesystem index test skipped")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; real-filesystem index test skipped")
	}
	// Pin the documented local default: a stray ZVEC_GREP_EMBEDDING in the
	// developer's environment must not change what this test builds with.
	t.Setenv("ZVEC_GREP_EMBEDDING", "")

	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	row := zvecGrepRow()
	row.Command = zg
	if _, err := rt.db.CreateMCPServer(context.Background(), row); err != nil {
		t.Fatalf("create zvec-grep server: %v", err)
	}

	prevLedger := indexLedger
	indexLedger = indexstate.New()
	t.Cleanup(func() { indexLedger = prevLedger })

	// Count runs while still executing the REAL indexer, so idempotency is
	// measured against actual `zg index` invocations.
	runs := new(atomic.Int32)
	prevRun := runZvecGrepIndex
	t.Cleanup(func() { runZvecGrepIndex = prevRun })
	runZvecGrepIndex = func(ctx context.Context, command, root, embedding string) ([]byte, error) {
		runs.Add(1)
		return prevRun(ctx, command, root, embedding)
	}
	return rt, runs
}

// newGitRepo makes a real `git init` repository holding one small source file.
func newGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	src := "package main\n\nfunc greet() string { return \"hello\" }\n"
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

// awaitSettled waits for a real run to leave PhaseIndexing and fails at once on
// PhaseFailed, reporting the indexer's reason instead of a timeout.
func awaitSettled(t *testing.T, root string) indexstate.Entry {
	t.Helper()
	deadline := time.Now().Add(realZvecGrepRunTimeout)
	for time.Now().Before(deadline) {
		e, seen := indexLedger.Get(exttoolsZvecGrepName, root)
		if !seen {
			t.Fatal("ledger lost the entry during the run")
		}
		switch e.Phase {
		case indexstate.PhaseReady:
			return e
		case indexstate.PhaseFailed:
			t.Fatalf("real index run failed: %s", e.Error)
		}
		time.Sleep(100 * time.Millisecond)
	}
	e, _ := indexLedger.Get(exttoolsZvecGrepName, root)
	t.Fatalf("real index run did not settle within %s (phase=%q)", realZvecGrepRunTimeout, e.Phase)
	return e
}

// TestManagedIndexRealCreateFromMissing drives the create path end to end on a
// real filesystem with the installed zg: nothing but the ledger is faked. It
// covers the path an existing index hides on a developer machine — including
// ensureZvecGrepGitExclude, which only runs when an index is created.
func TestManagedIndexRealCreateFromMissing(t *testing.T) {
	rt, runs := newRealZvecGrepRuntime(t)
	repo := newGitRepo(t)
	store := zvecGrepIndexPath(repo)

	// (a) missing: no store on disk, and the manager observes it as such.
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Fatalf("store exists before the first run: %v", err)
	}
	if got := obsPhase(rt.observeZvecGrep(repo)); got != indexstate.PhaseMissing {
		t.Fatalf("observed phase=%q before the first run, want missing", got)
	}

	if !rt.EnsureZvecGrepIndex(context.Background(), repo) {
		t.Fatal("no index scheduled for an unindexed git repository")
	}
	// (a) indexing: Begin claims synchronously and the real run takes seconds,
	// so the entry is still indexing when Ensure returns.
	e, seen := indexLedger.Get(exttoolsZvecGrepName, repo)
	if !seen {
		t.Fatal("no ledger entry after scheduling a create")
	}
	if e.Phase != indexstate.PhaseIndexing && e.Phase != indexstate.PhaseReady {
		t.Fatalf("phase=%q right after scheduling, want indexing", e.Phase)
	}
	if e.Action != indexstate.ActionCreate {
		t.Fatalf("action=%q, want create", e.Action)
	}

	// (a) ready.
	e = awaitSettled(t, repo)
	if e.Action != indexstate.ActionCreate || !e.Usable() {
		t.Fatalf("settled entry %+v, want a usable create", e)
	}
	if !strings.HasPrefix(e.Embedding, "local/") {
		t.Errorf("index built with %q — a background run must only use a local model", e.Embedding)
	}

	// (b) the store exists on disk, with the manifest the observer reads.
	if st, err := os.Stat(store); err != nil || !st.IsDir() {
		t.Fatalf("%s not created: %v", store, err)
	}
	manifest := zvecGrepManifestPath(repo)
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("manifest missing after a successful run: %v", err)
	}

	// (c) the create path wrote the exclude entry, and git honours it.
	exclude, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read .git/info/exclude: %v", err)
	}
	if !strings.Contains(string(exclude), zvecGrepIndexDir) {
		t.Fatalf(".git/info/exclude has no %s entry:\n%s", zvecGrepIndexDir, exclude)
	}
	status, err := exec.Command("git", "-C", repo, "status", "--porcelain", "--untracked-files=all").CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v: %s", err, status)
	}
	if bytes.Contains(status, []byte(zvecGrepIndexDir)) {
		t.Fatalf("index shows up as untracked files:\n%s", status)
	}

	// (d) ready is idempotent: a second call starts no run and leaves the store.
	if !rt.EnsureZvecGrepIndex(context.Background(), repo) {
		t.Fatal("second call reported no usable index")
	}
	if n := runs.Load(); n != 1 {
		t.Fatalf("zg index ran %d times, want 1", n)
	}
	if e2, _ := indexLedger.Get(exttoolsZvecGrepName, repo); e2.Phase != indexstate.PhaseReady {
		t.Fatalf("phase=%q after the second call, want ready", e2.Phase)
	}
	after, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("manifest changed on the second call — the index was rebuilt")
	}
	if n := strings.Count(string(exclude), zvecGrepIndexDir); n != 1 {
		t.Errorf("exclude entry written %d times, want 1", n)
	}
}

// TestManagedIndexRealSkipsScratchpad checks, with the real binary resolvable,
// that a throwaway working copy is still never indexed.
func TestManagedIndexRealSkipsScratchpad(t *testing.T) {
	rt, runs := newRealZvecGrepRuntime(t)
	pad := filepath.Join(newGitRepo(t), "scratchpad")
	if err := os.MkdirAll(pad, 0o755); err != nil {
		t.Fatal(err)
	}
	if rt.EnsureZvecGrepIndex(context.Background(), pad) {
		t.Fatal("a scratchpad was scheduled for indexing")
	}
	if n := runs.Load(); n != 0 {
		t.Fatalf("zg index ran %d times for a scratchpad", n)
	}
	if _, err := os.Stat(zvecGrepIndexPath(pad)); !os.IsNotExist(err) {
		t.Fatalf("scratchpad got an index store: %v", err)
	}
}
