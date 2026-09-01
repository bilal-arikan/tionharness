package agent

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// The four tests below all cover the same defect in four autonomous turn-entry
// paths (wake, scheduled prompt, automation fire, peer inbox delivery): the turn's
// cancel func was registered AFTER the per-session turn-slot claim, and the claim
// itself was uninterruptible. The claim can queue behind any other turn on the
// session for an unbounded time, so a "Durdur" landing in that window found nothing
// to cancel and the stopped turn started — and ran to completion — afterwards.
//
// Each test occupies the session's slot, waits until the turn under test is
// genuinely queued, then cancels it. Two things must hold: CancelSession must find
// the turn (registration happened BEFORE the claim), and the turn must record
// nothing afterwards (the claim was interruptible and the caller bailed).

// waitForQueuedKind blocks until a turn of kind is waiting in sessionID's queue,
// and reports whether it appeared before the deadline.
func waitForQueuedKind(rt *Runtime, sessionID string, kind turnqueue.Kind) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, w := range rt.TurnQueue().Snapshot(sessionID).Waiting {
			if w.Kind == kind {
				return true
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// turnSlotTestAgent creates a throwaway agent for the turn-entry paths under test.
func turnSlotTestAgent(t *testing.T, rt *Runtime, name string) db.Agent {
	t.Helper()
	ag, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: name, Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return ag
}

// TestWakeCancelledWhileQueuedNeverRuns: a wake stopped while it waits for its
// session's turn slot must not start afterwards, and must not write the wake prompt
// into the conversation it never answered.
func TestWakeCancelledWhileQueuedNeverRuns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()
	agent := turnSlotTestAgent(t, rt, "Bekçi")
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sc := db.Schedule{ID: "SCH1", AgentID: agent.ID, Prompt: "Devam et", SessionID: sess.ID, OneShot: true}

	// Occupy the session's turn slot so the wake has to queue behind it.
	releaseHolder := rt.claimSessionTurnSlot(sess.ID, turnqueue.KindUser, "test holder")
	defer releaseHolder()

	errCh := make(chan error, 1)
	go func() { errCh <- sched.deliverWake(ctx, sc) }()

	if !waitForQueuedKind(rt, sess.ID, turnqueue.KindWake) {
		t.Fatal("the wake turn never entered the session's turn queue")
	}
	if !rt.CancelSession(sess.ID) {
		t.Fatal("a queued wake must still be cancellable")
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, errWakeCancelledBeforeTurn) {
			t.Fatalf("expected errWakeCancelledBeforeTurn, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deliverWake outlived its own cancellation")
	}

	msgs, err := rt.db.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("a cancelled-while-queued wake must record nothing, got %d: %+v", len(msgs), msgs)
	}
}

// TestWakeCancelledBeforeTurnIsNotAFailedDelivery: fireWake must not park a
// "failure" delivery for a wake the user stopped. The failure path deliberately
// KEEPS the spent row so the fault stays visible in the routine list, so a retired
// row is the decisive evidence that this was not treated as a failure.
func TestWakeCancelledBeforeTurnIsNotAFailedDelivery(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()
	agent := turnSlotTestAgent(t, rt, "Bekçi")
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	armed, err := rt.db.CreateSchedule(ctx, db.Schedule{
		AgentID:   agent.ID,
		Prompt:    "Devam et",
		SessionID: sess.ID,
		OneShot:   true,
		FireAt:    time.Now().Add(time.Minute).Unix(),
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("create wake: %v", err)
	}

	releaseHolder := rt.claimSessionTurnSlot(sess.ID, turnqueue.KindUser, "test holder")
	defer releaseHolder()

	done := make(chan struct{})
	go func() { defer close(done); sched.fireWake(armed.ID) }()

	if !waitForQueuedKind(rt, sess.ID, turnqueue.KindWake) {
		t.Fatal("the wake turn never entered the session's turn queue")
	}
	if !rt.CancelSession(sess.ID) {
		t.Fatal("a queued wake must still be cancellable")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("fireWake outlived its own cancellation")
	}

	if _, err := rt.db.GetSchedule(ctx, armed.ID); err == nil {
		t.Fatal("a wake stopped before its turn must retire its row, not park a failure delivery")
	}
}

// TestScheduledTurnCancelledWhileQueuedNeverRuns: same defect on the cron
// scheduled-prompt path.
func TestScheduledTurnCancelledWhileQueuedNeverRuns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()
	agent := turnSlotTestAgent(t, rt, "Zamanlı")

	// Open the shared schedule thread up front so the test knows which session's
	// slot to occupy; deliverPrompt reuses the very same one.
	session, err := rt.db.GetOrCreateKindSession(ctx, agent.ID, "schedule", "⏰ Schedule")
	if err != nil {
		t.Fatalf("open schedule session: %v", err)
	}
	releaseHolder := rt.claimSessionTurnSlot(session.ID, turnqueue.KindUser, "test holder")
	defer releaseHolder()

	errCh := make(chan error, 1)
	go func() {
		_, derr := sched.deliverPrompt(ctx, db.Schedule{ID: "SCH2", AgentID: agent.ID, Prompt: "Raporu çıkar"})
		errCh <- derr
	}()

	if !waitForQueuedKind(rt, session.ID, turnqueue.KindWake) {
		t.Fatal("the scheduled turn never entered the session's turn queue")
	}
	if !rt.CancelSession(session.ID) {
		t.Fatal("a queued scheduled turn must still be cancellable")
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("a scheduled turn stopped while queued must report the stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deliverPrompt outlived its own cancellation")
	}

	msgs, err := rt.db.ListMessages(ctx, session.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("a cancelled-while-queued scheduled turn must record nothing, got %d: %+v", len(msgs), msgs)
	}
}

// TestScheduledTurnUntracksSessionWhenPromptAppendFails: deliverPrompt marks the
// session active BEFORE it claims the turn slot, so every bail after that point has
// to undo the marking. The prompt-append failure is the one that used to leak it,
// leaving the session permanently "running" and its agent permanently busy.
func TestScheduledTurnUntracksSessionWhenPromptAppendFails(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()
	agent := turnSlotTestAgent(t, rt, "Zamanlı")

	session, err := rt.db.GetOrCreateKindSession(ctx, agent.ID, "schedule", "⏰ Schedule")
	if err != nil {
		t.Fatalf("open schedule session: %v", err)
	}
	// Hold the slot so the turn is parked at the claim while the session is removed
	// underneath it; the prompt append then fails with ErrNotFound.
	releaseHolder := rt.claimSessionTurnSlot(session.ID, turnqueue.KindUser, "test holder")

	errCh := make(chan error, 1)
	go func() {
		_, derr := sched.deliverPrompt(ctx, db.Schedule{ID: "SCH3", AgentID: agent.ID, Prompt: "Raporu çıkar"})
		errCh <- derr
	}()

	if !waitForQueuedKind(rt, session.ID, turnqueue.KindWake) {
		t.Fatal("the scheduled turn never entered the session's turn queue")
	}
	if err := rt.db.DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	releaseHolder()

	select {
	case err := <-errCh:
		if !errors.Is(err, db.ErrNotFound) {
			t.Fatalf("expected the prompt append to fail with ErrNotFound, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deliverPrompt never returned")
	}

	if rt.isSessionActive(session.ID) {
		t.Fatal("a scheduled turn that failed to record its prompt must not leave the session tracked as active")
	}
}

// TestAutomationTurnCancelledWhileQueuedNeverRuns: same defect on the automation
// fire path. The bail must also return BEFORE the auto-continue / auto-handoff
// chain — a fire the user stopped must not resurrect itself as a follow-up turn.
func TestAutomationTurnCancelledWhileQueuedNeverRuns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agent := turnSlotTestAgent(t, rt, "Bakım")
	auto := db.Automation{ID: "AUT1", TargetAgentID: agent.ID}

	// Open the automation's persistent maintenance thread up front so the test knows
	// which session's slot to occupy; deliverAutomationTurn reuses the same one.
	session, err := rt.db.GetOrCreateSourceSession(ctx, SessionKindAutomation, auto.ID, agent.ID, "⚡ "+automationLabel(auto))
	if err != nil {
		t.Fatalf("open automation session: %v", err)
	}
	releaseHolder := rt.claimSessionTurnSlot(session.ID, turnqueue.KindUser, "test holder")
	defer releaseHolder()

	errCh := make(chan error, 1)
	go func() {
		_, derr := rt.deliverAutomationTurn(ctx, auto, "temizliği çalıştır")
		errCh <- derr
	}()

	if !waitForQueuedKind(rt, session.ID, turnqueue.KindAutomation) {
		t.Fatal("the automation turn never entered the session's turn queue")
	}
	if !rt.CancelSession(session.ID) {
		t.Fatal("a queued automation turn must still be cancellable")
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("an automation turn stopped while queued must report the stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deliverAutomationTurn outlived its own cancellation")
	}

	msgs, err := rt.db.ListMessages(ctx, session.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	// No prompt, no reply, and — crucially — no auto-continue follow-up turn.
	if len(msgs) != 0 {
		t.Fatalf("a cancelled-while-queued automation must record nothing, got %d: %+v", len(msgs), msgs)
	}
}

// TestPeerInboxDeliveryIsCancellableWhileQueued: the inbox delivery's cancel func
// used to be registered inside the delivery goroutine, after the turn-slot claim.
// send_agent_message returns to the sender's tool loop immediately, so a stop on
// the recipient's inbox in the very next iteration found nothing to cancel.
func TestPeerInboxDeliveryIsCancellableWhileQueued(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	sender := turnSlotTestAgent(t, rt, "Ada")
	target := turnSlotTestAgent(t, rt, "Kai")

	// Open the recipient's inbox up front so the test knows which session's slot to
	// occupy; deliverToInbox reuses the same one.
	inbox, err := rt.db.GetOrCreateSourceSession(ctx, inboxSessionKind, inboxSessionSource(target.ID), target.ID, inboxSessionTitle(target.Name))
	if err != nil {
		t.Fatalf("open inbox session: %v", err)
	}
	releaseHolder := rt.claimSessionTurnSlot(inbox.ID, turnqueue.KindUser, "test holder")
	defer releaseHolder()

	if err := rt.deliverToInbox(ctx, sender.ID, sender.Name, target, "özet", "işi devral"); err != nil {
		t.Fatalf("deliver to inbox: %v", err)
	}
	// No sleep, no polling: the registration must have happened-before the return.
	if !rt.CancelSession(inbox.ID) {
		t.Fatal("the inbox delivery was not cancellable at the moment the send returned")
	}

	// The cancelled delivery must release its slot without ever running the turn.
	drainSpawns(t, rt)
	msgs, err := rt.db.ListMessages(ctx, inbox.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	for _, m := range msgs {
		if m.Role == "assistant" {
			t.Fatalf("a cancelled-while-queued inbox delivery must record no reply, got %q", m.Text)
		}
	}
}

// The four tests below cover the last site of the same class: the coordinator drain
// loop. Its turn context was minted inside runCoordinatorTurn — i.e. AFTER the
// (uninterruptible) turn-slot claim had already returned — so a Stop landing while
// the drain waited in the admission queue found nothing to cancel, was swallowed,
// and the coordinator kept auto-turning.

// waitDrainStopped blocks until the drain loop has left the slot (driving cleared),
// so the assertions below observe the bail's final state and not the window before it.
func waitDrainStopped(t *testing.T, slot *coordSlot) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		slot.mu.Lock()
		driving := slot.driving
		slot.mu.Unlock()
		if !driving {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the bailed drain never cleared driving")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestCoordinatorDrainCancelledWhileQueuedNeverRuns: a drain turn stopped while it
// waits for the coordinator session's turn slot must be findable by CancelSession,
// must never run, and must leave the slot released.
func TestCoordinatorDrainCancelledWhileQueuedNeverRuns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	defer drainSpawns(t, rt)

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	// Occupy the coordinator's slot so the drain has to queue behind it.
	releaseHolder := rt.claimSessionTurnSlot("COORD", turnqueue.KindUser, "test holder")

	rt.enqueueCoordinatorTurn("COORD")
	if !waitForQueuedKind(rt, "COORD", turnqueue.KindCoordinator) {
		t.Fatal("the drain turn never entered the coordinator's turn queue")
	}
	if !rt.CancelSession("COORD") {
		t.Fatal("a queued coordinator drain turn must still be cancellable")
	}
	releaseHolder()

	// Give the (bailed) drain every chance to start the turn it must not start.
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	got := turns
	mu.Unlock()
	if got != 0 {
		t.Fatalf("a drain cancelled while queued must not run its turn; got %d turns", got)
	}
	if rt.sessionTurnBusy("COORD") {
		t.Fatal("the bailed drain must release the session's turn slot")
	}
}

// TestCoordinatorDrainBailDoesNotSpendBudgetOrEatTheNote: the bail must leave the
// auto-turn budget untouched (the turn never ran) and preserve slot.pending — the
// worker notification that armed the drain is still owed a turn.
func TestCoordinatorDrainBailDoesNotSpendBudgetOrEatTheNote(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.coordRunFn = func(string) {}
	slot := rt.coordSlotFor("COORD")
	// The preserved note is exactly what this test asserts, and it is also what
	// drainSpawns counts as unsettled background work — consume it after the
	// assertions so the cleanup sees an idle coordinator.
	defer func() {
		slot.mu.Lock()
		slot.pending = false
		slot.mu.Unlock()
		drainSpawns(t, rt)
	}()

	releaseHolder := rt.claimSessionTurnSlot("COORD", turnqueue.KindUser, "test holder")

	rt.enqueueCoordinatorTurn("COORD")
	if !waitForQueuedKind(rt, "COORD", turnqueue.KindCoordinator) {
		t.Fatal("the drain turn never entered the coordinator's turn queue")
	}
	// A second notification while the first is queued: this is the note that must
	// survive the stop.
	rt.enqueueCoordinatorTurn("COORD")
	if !rt.CancelSession("COORD") {
		t.Fatal("a queued coordinator drain turn must still be cancellable")
	}
	releaseHolder()
	waitDrainStopped(t, slot)

	slot.mu.Lock()
	spent, pending := slot.turns, slot.pending
	slot.mu.Unlock()
	if spent != 0 {
		t.Fatalf("a turn that never ran must not spend the auto-turn budget; slot.turns = %d", spent)
	}
	if !pending {
		t.Fatal("the bail must preserve the pending worker notification, not consume it")
	}
}

// TestCoordinatorDrainReArmsAfterQueuedStop: the bail must clear slot.driving, or
// every later notification takes the "already driving" branch and no drain ever
// starts again — a permanent, silent freeze.
func TestCoordinatorDrainReArmsAfterQueuedStop(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	defer drainSpawns(t, rt)

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	releaseHolder := rt.claimSessionTurnSlot("COORD", turnqueue.KindUser, "test holder")
	rt.enqueueCoordinatorTurn("COORD")
	if !waitForQueuedKind(rt, "COORD", turnqueue.KindCoordinator) {
		t.Fatal("the drain turn never entered the coordinator's turn queue")
	}
	if !rt.CancelSession("COORD") {
		t.Fatal("a queued coordinator drain turn must still be cancellable")
	}
	releaseHolder()
	waitDrainStopped(t, rt.coordSlotFor("COORD"))

	// A fresh worker notification after the stop must start a new drain and get its turn.
	rt.enqueueCoordinatorTurn("COORD")
	waitTurns(t, &mu, &turns, 1, "re-arm after a queued stop")
}

// TestCoordinatorDrainBailUntracksSession: the bail must drop the cancel func it
// registered before queueing. A leaked entry pins HasActiveSessions true forever —
// the workspace reads as permanently busy and a dead session stays "running".
func TestCoordinatorDrainBailUntracksSession(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	defer drainSpawns(t, rt)

	rt.coordRunFn = func(string) {}
	slot := rt.coordSlotFor("COORD")

	releaseHolder := rt.claimSessionTurnSlot("COORD", turnqueue.KindUser, "test holder")
	rt.enqueueCoordinatorTurn("COORD")
	if !waitForQueuedKind(rt, "COORD", turnqueue.KindCoordinator) {
		t.Fatal("the drain turn never entered the coordinator's turn queue")
	}
	if !rt.CancelSession("COORD") {
		t.Fatal("a queued coordinator drain turn must still be cancellable")
	}
	releaseHolder()
	waitDrainStopped(t, slot)

	if rt.CancelSession("COORD") {
		t.Fatal("the bailed drain must untrack the session, not leave its cancel registered")
	}
}

// waitForQueuedTurns blocks until at least n turns of kind are waiting in
// sessionID's queue, and reports whether they appeared before the deadline.
func waitForQueuedTurns(rt *Runtime, sessionID string, kind turnqueue.Kind, n int) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		seen := 0
		for _, w := range rt.TurnQueue().Snapshot(sessionID).Waiting {
			if w.Kind == kind {
				seen++
			}
		}
		if seen >= n {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// TestFinishedTurnKeepsTheQueuedTurnsRegistration: the flip side of the four tests
// above. Every autonomous entry path registers its cancel BEFORE it queues for the
// session's turn slot, and activeSessions holds ONE entry per session — so while
// turn A runs, turn B waiting behind it has already replaced A's registration with
// its own. A's exit must therefore leave the marker alone: an unconditional untrack
// (or one that lands after A released the slot) drops B's registration, and B then
// runs untracked — isSessionActive reads false and "Durdur" answers "not running"
// for a turn that is very much alive.
func TestFinishedTurnKeepsTheQueuedTurnsRegistration(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()
	agent := turnSlotTestAgent(t, rt, "Sıralı")

	// Open the shared schedule thread up front so the test knows which session's slot
	// to occupy; both deliverPrompt calls below reuse the very same one.
	session, err := rt.db.GetOrCreateKindSession(ctx, agent.ID, "schedule", "⏰ Schedule")
	if err != nil {
		t.Fatalf("open schedule session: %v", err)
	}
	// Hold the slot so both turns queue instead of running.
	releaseHolder := rt.claimSessionTurnSlot(session.ID, turnqueue.KindUser, "test holder")
	defer releaseHolder()

	// Turn A gets its own parent context: CancelSession is session-wide and would
	// only ever reach the LAST registration, so the test ends A through its own ctx.
	ctxA, cancelA := context.WithCancel(ctx)
	defer cancelA()
	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		_, _ = sched.deliverPrompt(ctxA, db.Schedule{ID: "SCHA", AgentID: agent.ID, Prompt: "A"})
	}()
	if !waitForQueuedTurns(rt, session.ID, turnqueue.KindWake, 1) {
		t.Fatal("turn A never entered the session's turn queue")
	}

	// Turn B queues behind A, registering its own cancel on the way in.
	bErr := make(chan error, 1)
	go func() {
		_, derr := sched.deliverPrompt(ctx, db.Schedule{ID: "SCHB", AgentID: agent.ID, Prompt: "B"})
		bErr <- derr
	}()
	if !waitForQueuedTurns(rt, session.ID, turnqueue.KindWake, 2) {
		t.Fatal("turn B never entered the session's turn queue")
	}

	// A ends. Everything below is about what its cleanup did to B's registration.
	cancelA()
	select {
	case <-aDone:
	case <-time.After(5 * time.Second):
		t.Fatal("turn A outlived its own cancellation")
	}

	if !rt.isSessionActive(session.ID) {
		t.Fatal("the finished turn evicted the queued turn's registration: the session reads idle while a turn is still queued")
	}
	if !rt.CancelSession(session.ID) {
		t.Fatal("the queued turn was no longer cancellable after the turn ahead of it finished")
	}
	select {
	case err := <-bErr:
		if err == nil {
			t.Fatal("the cancelled queued turn must report the stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("turn B outlived its cancellation")
	}
}
