package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// runtimeOverStore builds a Runtime on an EXPLICIT store directory, so a test can
// open two of them over the same on-disk state and exercise a restart.
// newTestRuntime cannot: it allocates a fresh t.TempDir() store per call, and its
// workDir argument is the sandbox root, not the store.
func runtimeOverStore(t *testing.T, storeDir, workDir string) *Runtime {
	t.Helper()
	database, err := db.Open(storeDir)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	rt := NewRuntime(database, providers.NewRegistry(), NewTunables(), workDir, filepath.Dir(workDir), nil, nil, "", "",
		nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	// Same fixture hygiene newTestRuntime applies, for the same reason: a fixture
	// turn ends in an error (no provider key), which in production fires the
	// hata→ders reflection — a real background turn that launches a claude-cli
	// subprocess and writes a claude-home under the fixture's TempDir. It then
	// holds that directory open past cleanup and Windows fails RemoveAll.
	rt.skipLessonDispatch = true
	// Registered AFTER the db-close cleanup so LIFO drains the background turns
	// first, then closes the db, then lets t.TempDir() remove the directory.
	t.Cleanup(func() {
		rt.CloseMCP()
		deadline := time.Now().Add(5 * time.Second)
		for (rt.spawnActive.Load() > 0 || rt.spawnQueueLen() > 0) && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
	})
	return rt
}

// seedSystemAgents installs the canonical built-ins into a test runtime's store,
// mirroring what workspace boot does.
func seedSystemAgents(t *testing.T, rt *Runtime) {
	t.Helper()
	if err := rt.db.EnsureSystemAgents(context.Background(), SystemAgentDefaults()...); err != nil {
		t.Fatalf("seed system agents: %v", err)
	}
}

// waitTurns blocks until *turns reaches want or the deadline elapses, guarding the
// counter with mu. Fails the test on timeout.
func waitTurns(t *testing.T, mu *sync.Mutex, turns *int, want int, what string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		got := *turns
		mu.Unlock()
		if got == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("%s: expected %d turns, got %d", what, want, got)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// waitForCondition polls ok until it holds or timeout elapses, failing the test
// with what on timeout.
func waitForCondition(t *testing.T, timeout time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for {
		if ok() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", what)
		case <-tick.C:
		}
	}
}

// profileAgent mirrors a built-in profile system agent ("subagent-explore") whose
// allowlist pins it to the profile's work tools.
func profileAgent(t *testing.T, allowed ...string) db.Agent {
	t.Helper()
	raw, err := json.Marshal(allowed)
	if err != nil {
		t.Fatalf("marshal allowlist: %v", err)
	}
	return db.Agent{
		Name:         "Worker: Explore",
		System:       true,
		SystemKey:    "subagent-explore",
		MCPEnabled:   true,
		AllowedTools: string(raw),
	}
}
