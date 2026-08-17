package agent

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// queueTestFixture spins up a runtime with a coordinator + one worker (backed by a
// real agent so SendToWorker's GetAgent succeeds) and installs the workerRunFn
// seam so delivery is captured instead of running a live provider turn. It returns
// the runtime, the ids, and a func that yields the captured delivery (message, ok).
func queueTestFixture(t *testing.T) (rt *Runtime, coordID, workerID string, delivered func() (string, bool)) {
	t.Helper()
	rt, _ = newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	// NotifyCoordinator enqueues a coordinator turn; stub it so failure paths don't
	// try to run a real provider turn.
	rt.coordRunFn = func(string) {}
	ctx := context.Background()

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	coord, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Role: "coordinator", SourceID: "s:c"})
	if err != nil {
		t.Fatalf("create coordinator: %v", err)
	}
	worker, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "worker", Role: "worker", SourceID: "s:w", CoordinatorSessionID: coord.ID})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	var mu sync.Mutex
	var got string
	var ok bool
	rt.workerRunFn = func(_ db.Agent, wsid, prompt, _ string) {
		mu.Lock()
		got, ok = prompt, wsid == worker.ID
		mu.Unlock()
	}
	return rt, coord.ID, worker.ID, func() (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		return got, ok
	}
}

// TestSendToWorkerQueuesWhenBusy: a follow-up to a worker mid-turn is parked in the
// single-slot queue and reported as queued (not rejected, not delivered yet).
func TestSendToWorkerQueuesWhenBusy(t *testing.T) {
	rt, coordID, workerID, delivered := queueTestFixture(t)
	ctx := context.Background()

	// Worker is mid-turn: isSessionActive == true, and a workerCtl so the elapsed
	// time is reported back.
	rt.trackSession(workerID, func() {})
	rt.workerCancels.Store(workerID, &workerCtl{startedAt: time.Now().Add(-3 * time.Second)})

	res, err := rt.SendToWorker(ctx, coordID, workerID, "second task")
	if err != nil {
		t.Fatalf("send to busy worker should queue, not error: %v", err)
	}
	if !res.Queued || res.Delivered {
		t.Fatalf("want Queued, got %+v", res)
	}
	if res.RunningForSeconds < 1 {
		t.Fatalf("queued result should carry the running turn's elapsed seconds, got %d", res.RunningForSeconds)
	}
	// The message is parked, and nothing was delivered while the turn is still busy.
	rt.workerQueueMu.Lock()
	parked := rt.workerQueue[workerID]
	rt.workerQueueMu.Unlock()
	if parked != "second task" {
		t.Fatalf("message not parked in queue, got %q", parked)
	}
	if _, ok := delivered(); ok {
		t.Fatal("nothing should be delivered while the worker is still running")
	}
}

// TestSendToWorkerRefusesSecondQueuedMessage: the queue is single-slot — a second
// pending message while the worker is busy is refused, and the first stays put.
func TestSendToWorkerRefusesSecondQueuedMessage(t *testing.T) {
	rt, coordID, workerID, _ := queueTestFixture(t)
	ctx := context.Background()

	rt.trackSession(workerID, func() {})

	if _, err := rt.SendToWorker(ctx, coordID, workerID, "first"); err != nil {
		t.Fatalf("first queue should succeed: %v", err)
	}
	res, err := rt.SendToWorker(ctx, coordID, workerID, "second")
	if err == nil {
		t.Fatalf("second queued message must be refused, got %+v", res)
	}
	// The first message is untouched.
	rt.workerQueueMu.Lock()
	parked := rt.workerQueue[workerID]
	rt.workerQueueMu.Unlock()
	if parked != "first" {
		t.Fatalf("first queued message must survive the refusal, got %q", parked)
	}
}

// TestDrainWorkerQueueDeliversOnTurnEnd: once the worker's turn ends (drain runs as
// runWorker's last deferred action), the parked message is popped and delivered as
// the next turn, and the queue slot frees up again.
func TestDrainWorkerQueueDeliversOnTurnEnd(t *testing.T) {
	rt, coordID, workerID, delivered := queueTestFixture(t)
	ctx := context.Background()
	agent, _ := rt.db.GetSession(ctx, workerID)
	wa, _ := rt.db.GetAgent(ctx, agent.AgentID)

	// Park a message while the worker is busy.
	rt.trackSession(workerID, func() {})
	if _, err := rt.SendToWorker(ctx, coordID, workerID, "queued task"); err != nil {
		t.Fatalf("queue: %v", err)
	}

	// Turn ends: isSessionActive flips false (untrackSession), then the drain runs.
	rt.untrackSession(workerID)
	rt.drainWorkerQueue(wa, workerID, coordID)

	msg, ok := delivered()
	if !ok || msg != "queued task" {
		t.Fatalf("drain should deliver the queued message to the worker, got (%q, %v)", msg, ok)
	}
	// Slot is empty again — a fresh follow-up could be queued next turn.
	rt.workerQueueMu.Lock()
	_, still := rt.workerQueue[workerID]
	rt.workerQueueMu.Unlock()
	if still {
		t.Fatal("queue slot must be freed after delivery")
	}
}

// TestSendToWorkerDeliversWhenIdle: the happy path — an idle worker gets the
// follow-up delivered straight away, no queue involved.
func TestSendToWorkerDeliversWhenIdle(t *testing.T) {
	rt, coordID, workerID, delivered := queueTestFixture(t)
	ctx := context.Background()

	res, err := rt.SendToWorker(ctx, coordID, workerID, "go now")
	if err != nil {
		t.Fatalf("idle send: %v", err)
	}
	if !res.Delivered || res.Queued {
		t.Fatalf("want Delivered, got %+v", res)
	}
	if msg, ok := delivered(); !ok || msg != "go now" {
		t.Fatalf("idle worker should get immediate delivery, got (%q, %v)", msg, ok)
	}
	rt.workerQueueMu.Lock()
	_, parked := rt.workerQueue[workerID]
	rt.workerQueueMu.Unlock()
	if parked {
		t.Fatal("idle delivery must not touch the queue")
	}
}
