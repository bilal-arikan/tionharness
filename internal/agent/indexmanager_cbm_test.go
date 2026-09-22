package agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// cliCall is one recorded codebase-memory CLI invocation.
type cliCall struct {
	tool    string
	project string
}

// cliRecorder records CLI calls made through the stubbed codebaseMemoryCLI.
type cliRecorder struct {
	mu    sync.Mutex
	calls []cliCall
}

func (c *cliRecorder) tools() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.calls))
	for i, call := range c.calls {
		out[i] = call.tool
	}
	return out
}

// stubCodebaseMemoryCLI replaces the codebase-memory CLI with a canned reply
// for every tool and returns the call recorder.
func stubCodebaseMemoryCLI(t *testing.T, reply string, err error) *cliRecorder {
	t.Helper()
	return stubCodebaseMemoryCLIFunc(t, func(string) (string, error) { return reply, err })
}

// stubCodebaseMemoryCLIFunc replaces the CLI with a per-tool reply function.
func stubCodebaseMemoryCLIFunc(t *testing.T, reply func(tool string) (string, error)) *cliRecorder {
	t.Helper()
	rec := &cliRecorder{}
	prev := codebaseMemoryCLI
	t.Cleanup(func() { codebaseMemoryCLI = prev })
	codebaseMemoryCLI = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		call := cliCall{tool: args[0]}
		for i := 1; i+1 < len(args); i++ {
			if args[i] == "--project" {
				call.project = args[i+1]
			}
		}
		rec.mu.Lock()
		rec.calls = append(rec.calls, call)
		rec.mu.Unlock()
		out, err := reply(args[0])
		return []byte(out), err
	}
	return rec
}

// newCodebaseMemoryRuntime builds a runtime with an enabled codebase-memory
// server, a private ledger and a stubbed index_repository whose result the test
// controls through the returned channel (nil error = success).
func newCodebaseMemoryRuntime(t *testing.T) (*Runtime, chan error) {
	t.Helper()
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	if _, err := rt.db.CreateMCPServer(context.Background(), db.MCPServer{
		Name: "codebase-memory-mcp", Transport: db.MCPTransportStdio,
		Command: "codebase-memory-mcp", Enabled: true,
	}); err != nil {
		t.Fatalf("create MCP server: %v", err)
	}
	prevLedger := indexLedger
	indexLedger = indexstate.New()
	t.Cleanup(func() { indexLedger = prevLedger })

	results := make(chan error, 4)
	prevRun := runIndexRepository
	t.Cleanup(func() { runIndexRepository = prevRun })
	runIndexRepository = func(ctx context.Context, _, _ string) ([]byte, error) {
		select {
		case err := <-results:
			if err != nil {
				return []byte("parse failure in main.go"), err
			}
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return rt, results
}

// awaitCodebaseMemoryRun polls the ledger until the entry leaves PhaseIndexing.
func awaitCodebaseMemoryRun(t *testing.T, root string) indexstate.Entry {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if e, seen := indexLedger.Get(codebaseMemoryToolName, root); seen && e.Phase != indexstate.PhaseIndexing {
			return e
		}
		time.Sleep(10 * time.Millisecond)
	}
	e, _ := indexLedger.Get(codebaseMemoryToolName, root)
	t.Fatalf("entry never settled: %+v", e)
	return e
}

func TestObserveCodebaseMemoryMapsTheServerReply(t *testing.T) {
	cases := []struct {
		reply   string
		exists  bool
		wantErr bool
	}{
		{`{"project":"p","status":"ready","nodes":3}`, true, false},
		{`{"project":"p","status":"not_found"}`, false, false},
		{`{"error":"project not found or not indexed","hint":"..."}`, false, false},
		// The real server writes an error reply to stderr between its log lines.
		{"level=warn msg=mem.allocator\nlevel=info msg=mem.init\n{\"project\":\"p\",\"status\":\"not_found\"}\n", false, false},
		// Anything unrecognised is an error: never a guessed ready.
		{`{"status":"indexing"}`, false, true},
		{`{"error":"store locked"}`, false, true},
		{`not json at all`, false, true},
	}
	for _, c := range cases {
		stubCodebaseMemoryCLI(t, c.reply, nil)
		obs, err := observeCodebaseMemory(context.Background(), "cbm", `C:\repo`)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v, wantErr=%v", c.reply, err, c.wantErr)
			continue
		}
		if err == nil && obs.Exists != c.exists {
			t.Errorf("%s: exists=%v, want %v", c.reply, obs.Exists, c.exists)
		}
	}
}

func TestEnsureCodebaseIndexedRecordsCreateToReady(t *testing.T) {
	rt, results := newCodebaseMemoryRuntime(t)
	rec := stubCodebaseMemoryCLI(t, `{"status":"not_found"}`, nil)
	repo := newRepo(t)

	results <- nil
	rt.EnsureCodebaseIndexed(context.Background(), repo)
	e := awaitCodebaseMemoryRun(t, repo)
	if e.Phase != indexstate.PhaseReady || e.Action != indexstate.ActionCreate {
		t.Fatalf("phase=%q action=%q, want ready/create", e.Phase, e.Action)
	}
	if got := rec.tools(); len(got) != 1 || got[0] != "index_status" {
		t.Errorf("CLI calls = %v, want one index_status observation", got)
	}
	if rec.calls[0].project != codebaseMemoryProjectID(repo) {
		t.Errorf("observed project %q, want %q", rec.calls[0].project, codebaseMemoryProjectID(repo))
	}
}

func TestEnsureCodebaseIndexedRecordsAFailureWithItsReasonAndAllowsRetry(t *testing.T) {
	rt, results := newCodebaseMemoryRuntime(t)
	stubCodebaseMemoryCLI(t, `{"status":"ready"}`, nil)
	repo := newRepo(t)

	results <- errors.New("exit status 2")
	rt.EnsureCodebaseIndexed(context.Background(), repo)
	e := awaitCodebaseMemoryRun(t, repo)
	if e.Phase != indexstate.PhaseFailed || e.Usable() {
		t.Fatalf("phase=%q usable=%v, want failed/false", e.Phase, e.Usable())
	}
	if !strings.Contains(e.Error, "exit status 2") || !strings.Contains(e.Error, "parse failure") {
		t.Errorf("reason=%q, want the error and the output tail", e.Error)
	}
	if e.Action != indexstate.ActionRefresh {
		t.Errorf("action=%q, want refresh for a project the store already held", e.Action)
	}
	if _, guarded := rt.cbmIndexed.Load(repo); guarded {
		t.Error("a failed run kept the per-runtime guard, so no later turn can retry")
	}
}

func TestEnsureCodebaseIndexedSkipsGuardedRoots(t *testing.T) {
	rt, _ := newCodebaseMemoryRuntime(t)
	rec := stubCodebaseMemoryCLI(t, `{"status":"not_found"}`, nil)
	for _, root := range []string{
		"relative/repo",
		filepath.Join(t.TempDir(), ".tionharness-worktrees", "WS1", "tsk1"),
		filepath.VolumeName(t.TempDir()) + string(filepath.Separator),
	} {
		rt.EnsureCodebaseIndexed(context.Background(), root)
		if _, seen := indexLedger.Get(codebaseMemoryToolName, root); seen {
			t.Errorf("%s: a guarded root reached the ledger", root)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if got := rec.tools(); len(got) != 0 {
		t.Errorf("guarded roots reached the CLI: %v", got)
	}
}

func TestRequestCodebaseMemoryRebuildDeletesThenIndexes(t *testing.T) {
	rt, results := newCodebaseMemoryRuntime(t)
	rec := stubCodebaseMemoryCLIFunc(t, func(tool string) (string, error) {
		if tool == "delete_project" {
			return `{"status":"deleted"}`, nil
		}
		return `{"status":"ready"}`, nil
	})
	repo := newRepo(t)

	results <- nil
	entry, err := rt.RequestIndexRun(context.Background(), IndexRequest{
		Tool: codebaseMemoryToolName, Root: repo, Action: indexstate.ActionRebuild,
	})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if entry.Phase != indexstate.PhaseIndexing || entry.Action != indexstate.ActionRebuild {
		t.Fatalf("claimed entry = %+v, want indexing/rebuild", entry)
	}
	if e := awaitCodebaseMemoryRun(t, repo); e.Phase != indexstate.PhaseReady {
		t.Fatalf("phase=%q, want ready", e.Phase)
	}
	if got := rec.tools(); len(got) != 1 || got[0] != "delete_project" {
		t.Errorf("CLI calls = %v, want delete_project before the index run", got)
	}
}

func TestRequestCodebaseMemoryRefreshOfAMissingProjectIsACreate(t *testing.T) {
	rt, results := newCodebaseMemoryRuntime(t)
	stubCodebaseMemoryCLI(t, `{"error":"project not found or not indexed"}`, errors.New("exit status 1"))
	repo := newRepo(t)

	results <- nil
	entry, err := rt.RequestIndexRun(context.Background(), IndexRequest{
		Tool: codebaseMemoryToolName, Root: repo, Action: indexstate.ActionRefresh,
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if entry.Action != indexstate.ActionCreate {
		t.Errorf("action=%q, want create", entry.Action)
	}
	awaitCodebaseMemoryRun(t, repo)
}

func TestRequestCodebaseMemoryRunIsSingleFlight(t *testing.T) {
	rt, results := newCodebaseMemoryRuntime(t)
	stubCodebaseMemoryCLI(t, `{"status":"ready"}`, nil)
	repo := newRepo(t)
	req := IndexRequest{Tool: codebaseMemoryToolName, Root: repo, Action: indexstate.ActionRefresh}

	if _, err := rt.RequestIndexRun(context.Background(), req); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if _, err := rt.RequestIndexRun(context.Background(), req); !errors.Is(err, ErrIndexRunInFlight) {
		t.Fatalf("second refresh err=%v, want ErrIndexRunInFlight", err)
	}
	results <- nil
	awaitCodebaseMemoryRun(t, repo)
}

func TestRequestCodebaseMemoryRunRefusesGuardedRoots(t *testing.T) {
	rt, _ := newCodebaseMemoryRuntime(t)
	stubCodebaseMemoryCLI(t, `{"status":"ready"}`, nil)
	ephemeral := filepath.Join(t.TempDir(), "scratchpad", "x")
	_, err := rt.RequestIndexRun(context.Background(), IndexRequest{
		Tool: codebaseMemoryToolName, Root: ephemeral, Action: indexstate.ActionRefresh,
	})
	if !errors.Is(err, ErrIndexRootNotAllowed) {
		t.Fatalf("err=%v, want ErrIndexRootNotAllowed", err)
	}
}

func TestDropCodebaseMemoryRequiresConfirmation(t *testing.T) {
	rt, _ := newCodebaseMemoryRuntime(t)
	rec := stubCodebaseMemoryCLI(t, `{"status":"deleted"}`, nil)
	repo := newRepo(t)
	indexLedger.Observe(codebaseMemoryToolName, repo, indexstate.PhaseReady, "", "")

	err := rt.DropSearchIndex(context.Background(), codebaseMemoryToolName, repo, "")
	if !errors.Is(err, indexstate.ErrDropNotConfirmed) {
		t.Fatalf("unconfirmed drop err=%v, want ErrDropNotConfirmed", err)
	}
	if len(rec.tools()) != 0 {
		t.Fatal("an unconfirmed drop reached the CLI")
	}

	if err := rt.DropSearchIndex(context.Background(), codebaseMemoryToolName, repo, repo); err != nil {
		t.Fatalf("confirmed drop: %v", err)
	}
	if got := rec.tools(); len(got) != 1 || got[0] != "delete_project" {
		t.Errorf("CLI calls = %v, want one delete_project", got)
	}
	if _, seen := indexLedger.Get(codebaseMemoryToolName, repo); seen {
		t.Error("a dropped index stayed in the ledger")
	}
}

func TestDropCodebaseMemoryRefusesARunInFlight(t *testing.T) {
	rt, _ := newCodebaseMemoryRuntime(t)
	rec := stubCodebaseMemoryCLI(t, `{"status":"deleted"}`, nil)
	repo := newRepo(t)
	indexLedger.Begin(codebaseMemoryToolName, repo, indexstate.ActionCreate)

	if err := rt.DropSearchIndex(context.Background(), codebaseMemoryToolName, repo, repo); !errors.Is(err, ErrIndexRunInFlight) {
		t.Fatalf("err=%v, want ErrIndexRunInFlight", err)
	}
	if len(rec.tools()) != 0 {
		t.Error("a drop under a live run reached the CLI")
	}
}

func TestDropCodebaseMemoryReportsACLIFailure(t *testing.T) {
	rt, _ := newCodebaseMemoryRuntime(t)
	stubCodebaseMemoryCLI(t, `{"error":"store is read-only"}`, errors.New("exit status 1"))
	repo := newRepo(t)
	indexLedger.Observe(codebaseMemoryToolName, repo, indexstate.PhaseReady, "", "")

	err := rt.DropSearchIndex(context.Background(), codebaseMemoryToolName, repo, repo)
	if err == nil || !strings.Contains(err.Error(), "store is read-only") {
		t.Fatalf("err=%v, want the server's reason", err)
	}
	if _, seen := indexLedger.Get(codebaseMemoryToolName, repo); !seen {
		t.Error("a failed drop forgot the ledger entry")
	}
}

func TestCodebaseMemoryStatusIsManagedAndReadsTheLedger(t *testing.T) {
	rt, _ := newCodebaseMemoryRuntime(t)
	stubCodebaseMemoryCLI(t, `{"status":"ready"}`, nil)
	repo := newRepo(t)
	c, _ := indexLedger.Begin(codebaseMemoryToolName, repo, indexstate.ActionCreate)
	if _, err := indexLedger.Fail(codebaseMemoryToolName, repo, c.Run, "exit status 2"); err != nil {
		t.Fatal(err)
	}

	st, ok := rt.codebaseMemoryStatus(context.Background(), repo)
	if !ok {
		t.Fatal("no codebase-memory row")
	}
	// The ledger's failure wins over a server that says ready: the last run failed.
	if !st.Managed || st.Phase != string(indexstate.PhaseFailed) || st.Error != "exit status 2" || st.Usable {
		t.Fatalf("status = %+v, want managed failed row with its reason", st)
	}
}

func TestCodebaseMemoryStatusObservesAnUnseenRootWithoutRecordingIt(t *testing.T) {
	rt, _ := newCodebaseMemoryRuntime(t)
	stubCodebaseMemoryCLI(t, `{"status":"ready"}`, nil)
	repo := newRepo(t)

	st, _ := rt.codebaseMemoryStatus(context.Background(), repo)
	if !st.Managed || st.Phase != string(indexstate.PhaseReady) || !st.Usable {
		t.Fatalf("status = %+v, want managed ready", st)
	}
	if _, seen := indexLedger.Get(codebaseMemoryToolName, repo); seen {
		t.Error("a status read wrote to the ledger")
	}

	stubCodebaseMemoryCLI(t, `garbage`, errors.New("exit status 3"))
	st, _ = rt.codebaseMemoryStatus(context.Background(), repo)
	if st.Phase != "unknown" || st.Usable || !strings.Contains(st.Note, "exit status 3") {
		t.Fatalf("status = %+v, want unknown with the reason", st)
	}
}
