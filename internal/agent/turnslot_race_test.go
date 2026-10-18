package agent

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
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

// TestFinishedTurnKeepsTheQueuedTurnsRegistration: the flip side of the tests
// above. Every autonomous entry path registers its cancel BEFORE it queues for the
// session's turn slot — so while turn A runs, turn B waiting behind it is already
// registered alongside it. A's exit must therefore leave B's entry alone: an
// unconditional untrack (or one that lands after A released the slot) drops B's
// registration, and B then
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

	// Turn A gets its own parent context: CancelSession is session-wide and now
	// cancels EVERY registration, so ending A through CancelSession would take B
	// down with it. The test ends A through its own ctx instead.
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
