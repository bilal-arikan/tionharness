package agent

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
)

func TestProjectIDForPath(t *testing.T) {
	cases := map[string]string{
		"":    "",
		"   ": "",
		`C:/Users/user/Desktop/Projects/TionHarness`:                "C-Users-user-Desktop-Projects-TionHarness",
		`C:\Users\user\Desktop\Projects\TionHarness`:                "C-Users-user-Desktop-Projects-TionHarness",
		`C:\Users\user\AppData\Local\Programs\@external-agentelectron`: "C-Users-user-AppData-Local-Programs-external-agentelectron",
		`/home/user/my-repo`:                                         "home-user-my-repo",
	}
	for in, want := range cases {
		if got := projectIDForPath(in); got != want {
			t.Errorf("projectIDForPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCodebaseMemoryCommand(t *testing.T) {
	stdio := db.MCPServer{Name: "cbm", Transport: db.MCPTransportStdio, Command: `C:\Progs\codebase-memory-mcp\codebase-memory-mcp.exe`}
	http := db.MCPServer{Name: "web", Transport: db.MCPTransportHTTP, URL: "http://x", Command: "codebase-memory-mcp"}
	other := db.MCPServer{Name: "brave", Transport: db.MCPTransportStdio, Command: "npx"}

	if got := codebaseMemoryCommand([]db.MCPServer{other, stdio}); got != stdio.Command {
		t.Errorf("expected stdio codebase-memory command, got %q", got)
	}
	if got := codebaseMemoryCommand([]db.MCPServer{other}); got != "" {
		t.Errorf("expected empty when absent, got %q", got)
	}
	// An http server that merely mentions the marker in Command must not match (the
	// auto-index helper shells out to a stdio executable only).
	if got := codebaseMemoryCommand([]db.MCPServer{http}); got != "" {
		t.Errorf("expected http transport to be skipped, got %q", got)
	}
}

func TestCodebaseMemoryGuidance(t *testing.T) {
	stdio := db.MCPServer{Name: "codebase-memory-mcp", Transport: db.MCPTransportStdio, Command: `C:\Progs\codebase-memory-mcp\codebase-memory-mcp.exe`}

	g := codebaseMemoryGuidance([]db.MCPServer{stdio}, mcp.ServerAlive)
	for _, want := range []string{
		"codebase-memory-mcp__search_code",
		"codebase-memory-mcp__search_graph",
		"codebase-memory-mcp__get_code_snippet",
		"codebase-memory-mcp__query_graph",
		"codebase-memory-mcp__trace_path",
		"codebase-memory-mcp__get_architecture",
		"codebase-memory-mcp__index_repository",
	} {
		if !strings.Contains(g, want) {
			t.Errorf("guidance missing namespaced tool %q\n%s", want, g)
		}
	}
	// Bare tool names must NOT appear — a bare name is what made models guess a
	// wrong namespace and hit "no server".
	for _, bare := range []string{" search_code ", " search_graph ", " index_repository "} {
		if strings.Contains(g, bare) {
			t.Errorf("guidance still exposes bare tool name %q\n%s", strings.TrimSpace(bare), g)
		}
	}

	// A verified-live pool connection is the only case allowed to assert the
	// connection outright.
	if !strings.Contains(g, "MCP server is connected.") {
		t.Errorf("alive state should assert the connection\n%s", g)
	}

	// Unverified (pool never dialed it — e.g. the claude-cli provider owns the
	// tool loop): the hint stays, but it must not claim a connection and must
	// name the fallback, so it cannot contradict the harness's own notices.
	u := codebaseMemoryGuidance([]db.MCPServer{stdio}, mcp.ServerUnknown)
	if strings.Contains(u, "MCP server is connected.") {
		t.Errorf("unverified state must not assert the connection\n%s", u)
	}
	for _, want := range []string{"configured", "Glob/Grep", "codebase-memory-mcp__search_code"} {
		if !strings.Contains(u, want) {
			t.Errorf("unverified guidance missing %q\n%s", want, u)
		}
	}

	// No server -> no hint at all (the capability block vanishes).
	if got := codebaseMemoryGuidance(nil, mcp.ServerAlive); got != "" {
		t.Errorf("expected empty guidance when no codebase-memory server, got %q", got)
	}
}

// TestCodebaseMemoryStateNilPool: a Runtime without a pool must report Unknown
// (not panic, not Dead) — Dead would silently delete a correct hint.
func TestCodebaseMemoryStateNilPool(t *testing.T) {
	r := &Runtime{}
	if got := r.codebaseMemoryState("codebase-memory-mcp"); got != mcp.ServerUnknown {
		t.Errorf("nil pool should be ServerUnknown, got %v", got)
	}
}

// cbmRuntime builds a Runtime whose codebase-memory feature toggle is on.
func cbmRuntime(t *testing.T) *Runtime {
	t.Helper()
	r := &Runtime{workDir: filepath.Join(t.TempDir(), "workspace")}
	r.codebaseMemoryEnabled.Store(true)
	return r
}

func TestEnsureCodebaseIndexedSkipsConcurrentSamePath(t *testing.T) {
	r1, _ := newTestRuntime(t, t.TempDir())
	r2, _ := newTestRuntime(t, t.TempDir())
	for _, r := range []*Runtime{r1, r2} {
		if _, err := r.db.CreateMCPServer(context.Background(), db.MCPServer{
			Name: "codebase-memory-mcp", Transport: db.MCPTransportStdio,
			Command: "codebase-memory-mcp", Enabled: true,
		}); err != nil {
			t.Fatalf("create MCP server: %v", err)
		}
	}

	original := runIndexRepository
	t.Cleanup(func() { runIndexRepository = original })
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var calls atomic.Int32
	runIndexRepository = func(_, _ string) ([]byte, error) {
		defer close(finished)
		calls.Add(1)
		close(started)
		<-release
		return nil, nil
	}

	repo := filepath.Join(t.TempDir(), "repo")
	r1.EnsureCodebaseIndexed(context.Background(), repo)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first index did not start")
	}
	r2.EnsureCodebaseIndexed(context.Background(), repo)
	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("index executions = %d, want 1", got)
	}
	close(release)
	<-finished
}

func TestEnsureCodebaseIndexedRunsDifferentPathsConcurrently(t *testing.T) {
	r, _ := newTestRuntime(t, t.TempDir())
	if _, err := r.db.CreateMCPServer(context.Background(), db.MCPServer{
		Name: "codebase-memory-mcp", Transport: db.MCPTransportStdio,
		Command: "codebase-memory-mcp", Enabled: true,
	}); err != nil {
		t.Fatalf("create MCP server: %v", err)
	}

	original := runIndexRepository
	t.Cleanup(func() { runIndexRepository = original })
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	var finished sync.WaitGroup
	finished.Add(2)
	runIndexRepository = func(_, _ string) ([]byte, error) {
		defer finished.Done()
		calls.Add(1)
		started <- struct{}{}
		<-release
		return nil, nil
	}

	root := t.TempDir()
	r.EnsureCodebaseIndexed(context.Background(), filepath.Join(root, "repo-a"))
	r.EnsureCodebaseIndexed(context.Background(), filepath.Join(root, "repo-b"))
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("only %d different-path indexes started", i)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("index executions = %d, want 2", got)
	}
	close(release)
	finished.Wait()
}
