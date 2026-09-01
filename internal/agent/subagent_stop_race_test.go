package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// TestSpawnSessionIsCancellableAsSoonAsItReturns: SpawnSession hands the session id
// to its caller, so the run must be stoppable from that instant. When the cancel
// func was registered inside the spawn goroutine, a stop issued in the same tool
// loop iteration found nothing to cancel and the caller was told the run had
// "already finished" while it kept going.
func TestSpawnSessionIsCancellableAsSoonAsItReturns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	res, err := rt.SpawnSession(ctx, a.ID, "Do the thing", SpawnOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// No sleep, no polling: the registration must have happened-before the return.
	if !rt.CancelSession(res.SessionID) {
		t.Fatal("the spawned run was not cancellable at the moment SpawnSession returned")
	}
	drainSpawns(t, rt)
}

// TestSpawnCancelledWhileQueuedNeverRuns: a spawn that is stopped while it waits
// for the session's turn slot must not start afterwards. The slot claim used to be
// uninterruptible (context.Background()), so a queued spawn outlived its own stop
// and ran to completion.
func TestSpawnCancelledWhileQueuedNeverRuns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	parentID, childID := subagentChild(t, rt, a.ID, runStateRunning)
	_ = parentID

	// Occupy the child's turn slot so the spawn turn has to queue behind it.
	releaseHolder := rt.claimSessionTurnSlot(childID, turnqueue.KindUser, "test holder")
	defer releaseHolder()

	if !rt.acquireSpawnSlot() {
		t.Fatal("could not reserve a spawn slot")
	}
	meta := db.Session{Kind: subagentSessionKind, ParentSessionID: parentID}
	runCtx, cancelRun := context.WithCancel(context.Background())
	rt.trackSession(childID, cancelRun)
	done := make(chan struct{})
	go func() {
		defer close(done)
		rt.runSpawn(runCtx, cancelRun, a, childID, "Do the thing", SpawnOptions{ChildSession: &meta})
	}()

	// Wait until the spawn turn is actually queued behind the holder, then stop it.
	deadline := time.Now().Add(5 * time.Second)
	for !spawnIsQueuedFor(rt, childID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !spawnIsQueuedFor(rt, childID) {
		t.Fatal("the spawn turn never entered the session's turn queue")
	}
	if !rt.CancelSession(childID) {
		t.Fatal("a queued spawn must still be cancellable")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled spawn did not return; the turn-slot claim ignored the stop")
	}

	got, err := rt.db.GetSession(ctx, childID)
	if err != nil {
		t.Fatalf("re-read child: %v", err)
	}
	if got.RunState != "killed" {
		t.Fatalf("expected runState killed, got %q", got.RunState)
	}
	msgs, err := rt.db.ListMessages(ctx, childID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	for _, m := range msgs {
		if m.Role == "assistant" {
			t.Fatalf("a cancelled-while-queued spawn must record no assistant turn, got %q", m.Text)
		}
	}
	drainSpawns(t, rt)
}

// spawnIsQueuedFor reports whether a spawn turn is waiting in sessionID's turn queue.
func spawnIsQueuedFor(rt *Runtime, sessionID string) bool {
	snap := rt.TurnQueue().Snapshot(sessionID)
	for _, w := range snap.Waiting {
		if w.Kind == turnqueue.KindSpawn {
			return true
		}
	}
	return false
}
