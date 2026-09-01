package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// TestCancelSessionCancelsEveryRegisteredTurn is the defect-B regression test. Every
// autonomous entry path registers its cancel BEFORE it queues for the session's turn
// slot, so a running turn and a queued one are registered at the same time. "Durdur"
// must reach BOTH — reaching only the newest registration is what let the user stop a
// session, get an OK and a "stopped" note in the transcript, and watch the RUNNING
// turn carry on.
func TestCancelSessionCancelsEveryRegisteredTurn(t *testing.T) {
	rt := &Runtime{}
	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	rt.trackSession("sess-1", cancelA)
	rt.trackSession("sess-1", cancelB)

	if !rt.CancelSession("sess-1") {
		t.Fatal("CancelSession returned false for a session with two registered turns")
	}
	select {
	case <-ctxA.Done():
	default:
		t.Fatal("the turn registered FIRST (the running one) was not cancelled")
	}
	select {
	case <-ctxB.Done():
	default:
		t.Fatal("the turn registered LAST (the queued one) was not cancelled")
	}
}

// TestUntrackRunLeavesTheOtherRegistrationAlone is the defect-A regression test at the
// data-structure level: a finishing turn releases its OWN handle, and the turn queued
// behind it stays registered — still visible as active, still stoppable.
func TestUntrackRunLeavesTheOtherRegistrationAlone(t *testing.T) {
	rt := &Runtime{}
	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	_, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	runA := rt.trackSession("sess-1", cancelA)
	runB := rt.trackSession("sess-1", cancelB)

	runB.release()
	if !rt.isSessionActive("sess-1") {
		t.Fatal("releasing one turn's handle emptied the session: it reads idle while another turn is live")
	}
	if !rt.CancelSession("sess-1") {
		t.Fatal("the surviving registration was no longer cancellable")
	}
	select {
	case <-ctxA.Done():
	default:
		t.Fatal("the surviving turn's context was not cancelled")
	}

	// Leak guard: once the last handle goes the session must read idle again, and a
	// second release must not resurrect or corrupt anything.
	runA.release()
	runA.release()
	if rt.isSessionActive("sess-1") {
		t.Fatal("the session still reads active after every registration was released")
	}
	if rt.HasActiveSessions() {
		t.Fatal("HasActiveSessions still true after the last registration was released")
	}
	if ids := rt.ActiveSessionIDs(); len(ids) != 0 {
		t.Fatalf("ActiveSessionIDs = %v, want empty", ids)
	}
}

// TestFinishedSpawnKeepsTheQueuedTurnsRegistration is the defect-A regression test on
// the spawn path: a spawn cancelled before its turn started must release only its own
// registration. Unconditional untracking there dropped the registration of whatever
// was queued behind it on the same session.
func TestFinishedSpawnKeepsTheQueuedTurnsRegistration(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "S", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, SourceID: "s:spawn"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Hold the slot so the spawn turn has to queue instead of running.
	releaseHolder := rt.claimSessionTurnSlot(session.ID, turnqueue.KindUser, "test holder")
	defer releaseHolder()

	// The turn queued BEHIND the spawn, registered like every entry path does.
	queuedCtx, cancelQueued := context.WithCancel(ctx)
	defer cancelQueued()
	rt.trackSession(session.ID, cancelQueued)

	if !rt.acquireSpawnSlot() {
		t.Fatal("could not reserve a spawn slot")
	}
	runCtx, cancelRun := context.WithCancel(context.Background())
	run := rt.trackSession(session.ID, cancelRun)
	done := make(chan struct{})
	go func() {
		defer close(done)
		rt.runSpawn(runCtx, cancelRun, run, agent, session.ID, "do the thing", SpawnOptions{})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !spawnIsQueuedFor(rt, session.ID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !spawnIsQueuedFor(rt, session.ID) {
		t.Fatal("the spawn turn never entered the session's turn queue")
	}

	// Stop the spawn through ITS OWN context, so nothing else is asked to cancel.
	cancelRun()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled spawn did not return")
	}

	if !rt.isSessionActive(session.ID) {
		t.Fatal("the finished spawn evicted the queued turn's registration: the session reads idle while a turn is still queued")
	}
	if !rt.CancelSession(session.ID) {
		t.Fatal("the queued turn was no longer cancellable after the spawn ahead of it ended")
	}
	select {
	case <-queuedCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("the queued turn's context was not cancelled")
	}
	drainSpawns(t, rt)
}

// TestFinishedWorkerKeepsTheQueuedTurnsRegistration is the defect-A regression test on
// the worker path: a worker turn stopped before its slot came free releases only the
// handle newWorkerRun put on its ctl, leaving whatever queued behind it registered.
func TestFinishedWorkerKeepsTheQueuedTurnsRegistration(t *testing.T) {
	rt, coordID, workerID, _ := queueTestFixture(t)
	ctx := context.Background()
	workerSession, err := rt.db.GetSession(ctx, workerID)
	if err != nil {
		t.Fatalf("get worker session: %v", err)
	}
	workerAgent, err := rt.db.GetAgent(ctx, workerSession.AgentID)
	if err != nil {
		t.Fatalf("get worker agent: %v", err)
	}

	// Hold the worker session's slot so the worker turn has to queue.
	releaseHolder := rt.claimSessionTurnSlot(workerID, turnqueue.KindUser, "test holder")
	defer releaseHolder()

	// The turn queued BEHIND the worker, registered like every entry path does.
	queuedCtx, cancelQueued := context.WithCancel(ctx)
	defer cancelQueued()
	rt.trackSession(workerID, cancelQueued)

	if !rt.acquireSpawnSlot() {
		t.Fatal("could not reserve a spawn slot")
	}
	runCtx, cancelRun, ctl := rt.newWorkerRun(workerID)
	cancelRun() // stopped while queued: the turn never runs
	rt.runWorkerWithCtl(runCtx, cancelRun, workerAgent, workerID, "do the thing", coordID, ctl)

	if !rt.isSessionActive(workerID) {
		t.Fatal("the finished worker evicted the queued turn's registration: the session reads idle while a turn is still queued")
	}
	if rt.workerTurnActive(workerID) {
		t.Fatal("the worker's own registration leaked: the backpressure gate would park follow-ups forever")
	}
	if !rt.CancelSession(workerID) {
		t.Fatal("the queued turn was no longer cancellable after the worker ahead of it ended")
	}
	select {
	case <-queuedCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("the queued turn's context was not cancelled")
	}
	drainSpawns(t, rt)
}

// TestInboxDeliveryUntracksOnlyItsOwnRegistration pins the IDENTITY scope of the peer
// inbox delivery's two untrack sites (the cancelled-before-the-slot one and the
// after-the-turn one): both must remove only the delivery's own handle. Untracking by
// session id there would drop the registration of any turn queued behind the delivery
// on the same inbox session.
func TestInboxDeliveryUntracksOnlyItsOwnRegistration(t *testing.T) {
	cases := []struct {
		name string
		// queued=true holds the inbox session's turn slot, so the delivery never runs
		// its turn and exits through the cancelled-before-the-slot path.
		queued bool
	}{
		{name: "cancelled before its turn started", queued: true},
		{name: "after the turn ran", queued: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
			ctx := context.Background()
			agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "P", Provider: "anthropic"})
			if err != nil {
				t.Fatalf("create agent: %v", err)
			}
			inbox, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, SourceID: "s:inbox"})
			if err != nil {
				t.Fatalf("create inbox session: %v", err)
			}

			// A foreign turn registered on the same inbox session, standing in for the
			// turn queued behind the delivery.
			foreignCtx, cancelForeign := context.WithCancel(ctx)
			defer cancelForeign()
			rt.trackSession(inbox.ID, cancelForeign)

			if !rt.acquireSpawnSlot() {
				t.Fatal("could not reserve a spawn slot")
			}
			runCtx, cancelRun := context.WithCancel(context.Background())
			defer cancelRun()
			run := rt.trackSession(inbox.ID, cancelRun)

			if tc.queued {
				releaseHolder := rt.claimSessionTurnSlot(inbox.ID, turnqueue.KindUser, "test holder")
				defer releaseHolder()
				cancelRun() // the claim below then fails immediately
			}
			// No provider is configured, so the turn itself fails fast; the untrack site
			// under test runs either way.
			rt.runInboxDelivery(runCtx, cancelRun, run, agent, inbox.ID, "hello")

			if !rt.isSessionActive(inbox.ID) {
				t.Fatal("the delivery evicted the foreign registration: the session reads idle while a turn is still registered")
			}
			if !rt.CancelSession(inbox.ID) {
				t.Fatal("the foreign turn was no longer cancellable after the delivery ended")
			}
			select {
			case <-foreignCtx.Done():
			case <-time.After(time.Second):
				t.Fatal("the foreign turn's context was not cancelled")
			}
			// Leak guard: the delivery's OWN handle must be gone.
			cancelForeign()
			rt.untrackSessionRun(inbox.ID, nil)
			if rt.isSessionActive(inbox.ID) {
				t.Fatal("registrations survived a full teardown")
			}
			drainSpawns(t, rt)
		})
	}
}

// TestCoordinatorDrainIterationUntracksOnlyItsOwn: a drain iteration ends its turn
// context through endTurnCtx, which must release only that iteration's registration.
// Clearing the session's marker there dropped any foreign turn registered on the
// coordinator session (a user turn, a wake) and left it unstoppable.
func TestCoordinatorDrainIterationUntracksOnlyItsOwn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	foreignCtx, cancelForeign := context.WithCancel(context.Background())
	defer cancelForeign()
	rt.trackSession("COORD", cancelForeign)

	started := make(chan struct{})
	release := make(chan struct{})
	rt.coordRunFn = func(string) {
		started <- struct{}{}
		<-release
	}
	rt.enqueueCoordinatorTurn("COORD")
	<-started
	release <- struct{}{}

	// Wait for the drain goroutine to finish the iteration and settle.
	slot := rt.coordSlotFor("COORD")
	deadline := time.Now().Add(5 * time.Second)
	for {
		slot.mu.Lock()
		driving := slot.driving
		slot.mu.Unlock()
		if !driving {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the coordinator drain never settled")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if !rt.isSessionActive("COORD") {
		t.Fatal("the drain iteration evicted the foreign registration: the coordinator session reads idle while a turn is still registered")
	}
	if !rt.CancelSession("COORD") {
		t.Fatal("the foreign turn was no longer cancellable after the drain iteration ended")
	}
	select {
	case <-foreignCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("the foreign turn's context was not cancelled")
	}
	// Leak guard: the drain's own registration must be gone, so releasing the foreign
	// one leaves the session idle.
	rt.untrackSession("COORD")
	if rt.isSessionActive("COORD") {
		t.Fatal("registrations survived a full teardown")
	}
}

// TestAutoContinueIterationReleasesItsRegistration pins the auto-continue loop's
// per-iteration registration: the handle trackSession hands out must be released when
// the iteration's turn ends, on BOTH the normal return and the panic path. Since the
// registry keeps a LIST per session, a missed release is permanent — the session would
// read "running" forever, the agent stay busy forever, and the coordinator budget hold
// a dead slot.
func TestAutoContinueIterationReleasesItsRegistration(t *testing.T) {
	cases := []struct {
		name    string
		panics  bool
		wantErr bool
	}{
		{name: "turn returns normally"},
		{name: "turn returns an error", wantErr: true},
		{name: "turn panics", panics: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Runtime{}

			// A foreign turn registered on the same session, standing in for the outer
			// scheduled/spawn registration the caller is still holding: the iteration
			// must release ONLY its own handle.
			foreignCtx, cancelForeign := context.WithCancel(context.Background())
			defer cancelForeign()
			rt.trackSession("SESS", cancelForeign)

			runCtx, cancelRun := context.WithCancel(context.Background())
			defer cancelRun()
			ran := false
			call := func() {
				if tc.panics {
					defer func() { _ = recover() }()
				}
				_, _, err := rt.runTrackedContinuation("SESS", cancelRun, func() (string, []TurnStep, error) {
					ran = true
					if tc.panics {
						panic("turn blew up")
					}
					if tc.wantErr {
						return "", nil, context.Canceled
					}
					return "ok", []TurnStep{{Kind: StepText}}, nil
				})
				if tc.wantErr && err == nil {
					t.Error("the turn's error was swallowed")
				}
			}
			call()
			if !ran {
				t.Fatal("the turn never ran")
			}

			// The iteration's own context is cancelled and its registration is gone,
			// while the foreign one survives.
			select {
			case <-runCtx.Done():
			default:
				t.Error("the iteration's turn context was not cancelled")
			}
			if !rt.isSessionActive("SESS") {
				t.Fatal("the iteration released the foreign registration too: the session reads idle while a turn is still live")
			}
			// Exactly the foreign handle is left: the iteration's own registration must
			// not linger, or the session stays "running" forever.
			rt.activeMu.Lock()
			left := len(rt.activeSessions["SESS"])
			rt.activeMu.Unlock()
			if left != 1 {
				t.Fatalf("registrations left for the session = %d, want 1 (only the foreign one)", left)
			}
			if !rt.CancelSession("SESS") {
				t.Fatal("the foreign turn was no longer cancellable after the iteration ended")
			}
			select {
			case <-foreignCtx.Done():
			case <-time.After(time.Second):
				t.Fatal("the surviving registration is not the foreign turn's")
			}
		})
	}
}

// TestWorkerBackpressureIgnoresForeignRegistration pins the decision taken for the
// send_to_worker backpressure gate: it keys off a WORKER registration, not "the
// session has any registration". Only a worker turn is paired with drainWorkerQueue,
// so a wake/user/peer turn registered on the worker session must not make a follow-up
// look queueable — it would be parked with nobody left to drain it.
func TestWorkerBackpressureIgnoresForeignRegistration(t *testing.T) {
	rt, coordID, workerID, delivered := queueTestFixture(t)
	ctx := context.Background()

	// A foreign (non-worker) turn on the worker session: no worker turn is running.
	_, cancelForeign := context.WithCancel(ctx)
	defer cancelForeign()
	rt.trackSession(workerID, cancelForeign)

	res, err := rt.SendToWorker(ctx, coordID, workerID, "go now")
	if err != nil {
		t.Fatalf("send with only a foreign registration: %v", err)
	}
	if !res.Delivered || res.Queued {
		t.Fatalf("want Delivered (no worker turn is live), got %+v", res)
	}
	if msg, ok := delivered(); !ok || msg != "go now" {
		t.Fatalf("follow-up should be dispatched, got (%q, %v)", msg, ok)
	}
	rt.workerQueueMu.Lock()
	_, parked := rt.workerQueue[workerID]
	rt.workerQueueMu.Unlock()
	if parked {
		t.Fatal("a follow-up was parked behind a turn that will never drain the queue")
	}
}
