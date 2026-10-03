package agent

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// The workspace manager closes the DB the instant Runtime.CloseMCP returns, so a
// background turn that is still running would write into a closed store. These
// tests pin the barrier that makes that impossible.

// TestCloseMCP_WaitsForInFlightBackgroundTurn proves CloseMCP does not return
// while a background turn is still unwinding.
func TestCloseMCP_WaitsForInFlightBackgroundTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	release := make(chan struct{})
	var finished atomic.Bool
	if !rt.startBackgroundTurn(func() {
		<-release
		finished.Store(true)
	}) {
		t.Fatal("startBackgroundTurn refused on a live runtime")
	}

	// Let the turn finish only after CloseMCP has had time to start waiting, so a
	// CloseMCP that does not wait is caught by the flag below.
	go func() {
		time.Sleep(150 * time.Millisecond)
		close(release)
	}()

	rt.CloseMCP()
	if !finished.Load() {
		t.Fatal("CloseMCP returned while a background turn was still running")
	}
	drainSpawns(t, rt)
}

// TestCloseMCP_RejectsLaterSpawns proves the door is shut for new work: a spawn
// that finds a free slot must not reach launchSpawn (and its CreateSession write)
// after the workspace has started closing.
func TestCloseMCP_RejectsLaterSpawns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Worker", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rt.CloseMCP()

	res, err := rt.SpawnSession(ctx, agent.ID, "Do the thing", SpawnOptions{CreatedBy: agent.ID})
	if err == nil {
		t.Fatalf("SpawnSession succeeded after CloseMCP (session %q)", res.SessionID)
	}
	if !errors.Is(err, errSpawnQueueShutdown) {
		t.Fatalf("error = %v, want %v", err, errSpawnQueueShutdown)
	}
	drainSpawns(t, rt)
}

// TestCloseMCP_ReturnsAfterGraceWhenTurnIsStuck proves the wait is BOUNDED: a turn
// that never unwinds delays the shutdown by the grace, it does not hang it forever.
func TestCloseMCP_ReturnsAfterGraceWhenTurnIsStuck(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.spawnDrainGrace = 200 * time.Millisecond

	stuck := make(chan struct{})
	// Released here rather than in the turn itself: the goroutine must outlive
	// CloseMCP for the timeout branch to be exercised at all, but it must NOT
	// outlive the test, or the t.TempDir() cleanup races it.
	defer close(stuck)
	if !rt.startBackgroundTurn(func() { <-stuck }) {
		t.Fatal("startBackgroundTurn refused on a live runtime")
	}

	done := make(chan struct{})
	go func() {
		rt.CloseMCP()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseMCP did not return within the drain grace")
	}
}
