package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// TestSpawnWorkerIsCancellableAsSoonAsItReturns: SpawnWorker hands the worker
// session id to the coordinator's tool loop, so stop_worker in the very next
// iteration must find something to cancel. When the ctl and the session cancel were
// registered inside the worker goroutine, that stop found nothing and stop_worker
// reported "already finished" for a worker that then went on to run.
func TestSpawnWorkerIsCancellableAsSoonAsItReturns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	coord := newTestCoordinator(t, rt, 0)

	res, err := rt.SpawnWorker(ctx, coord, "Coord0", "do the thing", "", WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	// No sleep, no polling: both registrations must have happened-before the return.
	if _, ok := rt.workerCancels.Load(res.SessionID); !ok {
		t.Error("stop_worker had no workerCtl at the moment SpawnWorker returned")
	}
	if !rt.CancelSession(res.SessionID) {
		t.Fatal("the spawned worker was not cancellable at the moment SpawnWorker returned")
	}
	drainSpawns(t, rt)
}

// TestWorkerCancelledWhileQueuedNeverRuns: a worker stopped while it waits for its
// session's turn slot must not start afterwards. The slot claim used to be
// uninterruptible (context.Background()), so a queued worker outlived its own stop
// and ran to completion — with the window unbounded, since the claim can queue
// behind any other turn on the worker session.
func TestWorkerCancelledWhileQueuedNeverRuns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	coord := newTestCoordinator(t, rt, 0)
	ag, workerID := workerUnderCoordinator(t, rt, coord, runStateRunning)

	// Occupy the worker session's turn slot so the worker turn has to queue behind it.
	releaseHolder := rt.claimSessionTurnSlot(workerID, turnqueue.KindUser, "test holder")
	defer releaseHolder()

	if !rt.acquireSpawnSlot() {
		t.Fatal("could not reserve a spawn slot")
	}
	runCtx, cancelRun, ctl := rt.newWorkerRun(workerID)
	done := make(chan struct{})
	go func() {
		defer close(done)
		rt.runWorkerRegistered(runCtx, cancelRun, ag, workerID, "do the thing", coord, ctl)
	}()

	// Wait until the worker turn is actually queued behind the holder, then stop it.
	deadline := time.Now().Add(5 * time.Second)
	for !workerIsQueuedFor(rt, workerID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !workerIsQueuedFor(rt, workerID) {
		t.Fatal("the worker turn never entered the session's turn queue")
	}
	if !rt.CancelSession(workerID) {
		t.Fatal("a queued worker must still be cancellable")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled worker did not return; the turn-slot claim ignored the stop")
	}

	got, err := rt.db.GetSession(ctx, workerID)
	if err != nil {
		t.Fatalf("re-read worker: %v", err)
	}
	// "killed", not "failed": a later retry_of must not be refused as still running.
	if got.RunState != turnStatusKilled {
		t.Fatalf("expected runState %q, got %q", turnStatusKilled, got.RunState)
	}
	msgs, err := rt.db.ListMessages(ctx, workerID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	for _, m := range msgs {
		if m.Role == "assistant" {
			t.Fatalf("a cancelled-while-queued worker must record no assistant turn, got %q", m.Text)
		}
	}

	// The coordinator is waiting on this worker whatever happened to it, so the kill
	// must still be reported: dropping the notification freezes the coordinator on a
	// worker that will never speak.
	coordMsgs, err := rt.db.ListMessages(ctx, coord)
	if err != nil {
		t.Fatalf("list coordinator messages: %v", err)
	}
	found := false
	for _, m := range coordMsgs {
		if m.Origin != "worker-note" {
			continue
		}
		if !strings.Contains(m.Text, "<task-id>"+workerID+"</task-id>") {
			continue
		}
		found = true
		if !strings.Contains(m.Text, "<status>"+turnStatusKilled+"</status>") {
			t.Fatalf("the coordinator's note for the queued-cancelled worker must report %q, got %q", turnStatusKilled, m.Text)
		}
	}
	if !found {
		t.Fatalf("the coordinator got no task-notification for worker %s cancelled while queued", workerID)
	}
	drainSpawns(t, rt)
}

// TestStopWorkerUntrackedRunningIsAnError: a worker still marked "running" with no
// cancellable turn is NOT the "it already finished" race — claiming so is a plain
// false statement, and it leaves the coordinator unable to stop and unable to retry.
func TestStopWorkerUntrackedRunningIsAnError(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	coord := newTestCoordinator(t, rt, 0)
	_, workerID := workerUnderCoordinator(t, rt, coord, runStateRunning)

	err := rt.StopWorker(ctx, coord, workerID)
	if err == nil || !strings.Contains(err.Error(), "no cancellable turn") {
		t.Fatalf("expected an explicit no-cancellable-turn error, got %v", err)
	}
}

// workerUnderCoordinator creates a worker session owned by coordSessionID, stamped
// with runState when non-empty, and returns its agent plus the session id.
func workerUnderCoordinator(t *testing.T, rt *Runtime, coordSessionID, runState string) (db.Agent, string) {
	t.Helper()
	ctx := context.Background()
	ag, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create worker agent: %v", err)
	}
	worker, err := rt.db.CreateSession(ctx, db.Session{
		AgentID:              ag.ID,
		Kind:                 "worker",
		Role:                 "worker",
		SourceID:             "test:worker:" + coordSessionID,
		CoordinatorSessionID: coordSessionID,
	})
	if err != nil {
		t.Fatalf("create worker session: %v", err)
	}
	if runState != "" {
		if err := rt.db.SetSessionRunState(ctx, worker.ID, runState, time.Now().Unix()); err != nil {
			t.Fatalf("set run state: %v", err)
		}
	}
	return ag, worker.ID
}

// workerIsQueuedFor reports whether a worker turn is waiting in sessionID's queue.
func workerIsQueuedFor(rt *Runtime, sessionID string) bool {
	snap := rt.TurnQueue().Snapshot(sessionID)
	for _, w := range snap.Waiting {
		if w.Kind == turnqueue.KindWorker {
			return true
		}
	}
	return false
}
