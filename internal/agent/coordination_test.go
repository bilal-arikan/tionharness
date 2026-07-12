package agent

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestFormatTaskNotification checks the coordinator-facing XML carries the
// worker id, status, and result.
func TestFormatTaskNotification(t *testing.T) {
	note := formatTaskNotification("SES9", "Scout", "completed", "found it in foo.go:42", 3, 1200)
	for _, want := range []string{
		"<task-notification>", "<task-id>SES9</task-id>", "<status>completed</status>",
		"found it in foo.go:42", "<tool_uses>3</tool_uses>", "<duration_ms>1200</duration_ms>",
	} {
		if !strings.Contains(note, want) {
			t.Errorf("notification missing %q:\n%s", want, note)
		}
	}
}

// TestCountToolSteps counts only tool steps.
func TestCountToolSteps(t *testing.T) {
	steps := []TurnStep{
		{Kind: StepText}, {Kind: StepTool}, {Kind: StepThinking}, {Kind: StepTool},
	}
	if n := countToolSteps(steps); n != 2 {
		t.Fatalf("countToolSteps = %d, want 2", n)
	}
}

// TestCoordinatorQueueSerializesAndCoalesces is the critical race guard: two
// notifications that arrive while a coordinator turn is running must (a) never run
// two turns concurrently, and (b) coalesce into exactly ONE follow-up turn (they
// are already persisted, so the next turn sees them all).
func TestCoordinatorQueueSerializesAndCoalesces(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	concurrent, maxConcurrent, turns := 0, 0, 0
	started := make(chan struct{})
	release := make(chan struct{})
	rt.coordRunFn = func(string) {
		mu.Lock()
		concurrent++
		if concurrent > maxConcurrent {
			maxConcurrent = concurrent
		}
		turns++
		mu.Unlock()
		started <- struct{}{}
		<-release
		mu.Lock()
		concurrent--
		mu.Unlock()
	}

	// First enqueue starts turn 1.
	rt.enqueueCoordinatorTurn("COORD")
	<-started

	// Two more notifications arrive WHILE turn 1 runs → they must coalesce (pending),
	// not spawn new turns.
	rt.enqueueCoordinatorTurn("COORD")
	rt.enqueueCoordinatorTurn("COORD")

	// Let turn 1 finish; the pending flag triggers exactly one follow-up (turn 2).
	release <- struct{}{}
	<-started
	release <- struct{}{}

	// Give the drain goroutine a moment to settle (no third turn should appear).
	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		done := turns == 2
		mu.Unlock()
		if done {
			break
		}
		select {
		case <-deadline:
			mu.Lock()
			got := turns
			mu.Unlock()
			t.Fatalf("expected exactly 2 coordinator turns (coalesced), got %d", got)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if maxConcurrent != 1 {
		t.Fatalf("coordinator turns overlapped: maxConcurrent = %d, want 1", maxConcurrent)
	}
}

// TestUserTurnBlocksAutoTurnsAndDrainsPending: while an interactive (user) turn
// holds the coordinator slot, worker notifications must NOT start an auto turn —
// they fall into pending — and releasing the user turn must run exactly ONE
// coalesced follow-up turn.
func TestUserTurnBlocksAutoTurnsAndDrainsPending(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) {
		mu.Lock()
		turns++
		mu.Unlock()
	}

	release := rt.BeginCoordinatorUserTurn("COORD")

	// Two notifications land mid-user-turn: no auto turn may start.
	rt.enqueueCoordinatorTurn("COORD")
	rt.enqueueCoordinatorTurn("COORD")
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if turns != 0 {
		mu.Unlock()
		t.Fatalf("auto turn ran while user turn held the slot: turns = %d", turns)
	}
	mu.Unlock()

	// Releasing the user turn drains the pile-up into exactly one turn.
	release()
	deadline := time.After(time.Second)
	for {
		mu.Lock()
		n := turns
		mu.Unlock()
		if n == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected exactly 1 coalesced turn after release, got %d", n)
		case <-time.After(10 * time.Millisecond):
		}
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if turns != 1 {
		t.Fatalf("expected exactly 1 coalesced turn, got %d", turns)
	}
}

// TestUserTurnWaitsForAutoTurnAndResetsCap: a user turn arriving while an auto
// turn runs must block until it finishes, and claiming the slot resets the
// auto-turn cap (human back in the loop).
func TestUserTurnWaitsForAutoTurnAndResetsCap(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	started := make(chan struct{})
	releaseAuto := make(chan struct{})
	rt.coordRunFn = func(string) {
		started <- struct{}{}
		<-releaseAuto
	}

	// Pre-load cap state to verify the reset.
	slot := rt.coordSlotFor("COORD")
	slot.mu.Lock()
	slot.turns = 40
	slot.capWarn = true
	slot.mu.Unlock()

	rt.enqueueCoordinatorTurn("COORD")
	<-started

	acquired := make(chan func(), 1)
	go func() { acquired <- rt.BeginCoordinatorUserTurn("COORD") }()

	select {
	case <-acquired:
		t.Fatal("user turn acquired the slot while an auto turn was running")
	case <-time.After(50 * time.Millisecond):
	}

	releaseAuto <- struct{}{}
	var release func()
	select {
	case release = <-acquired:
	case <-time.After(time.Second):
		t.Fatal("user turn never acquired the slot after the auto turn finished")
	}

	slot.mu.Lock()
	turnsAfter, warnAfter := slot.turns, slot.capWarn
	slot.mu.Unlock()
	if turnsAfter != 0 || warnAfter {
		t.Fatalf("user turn should reset cap state, got turns=%d capWarn=%v", turnsAfter, warnAfter)
	}
	release()
}

// TestClaimTurnSlotIfCoordinator: a coordinator session's autonomous turn claims
// the slot (blocking auto turns into pending, without resetting the cap); a
// non-coordinator session gets a no-op release even when a slot with the same id
// happens to be busy.
func TestClaimTurnSlotIfCoordinator(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "C", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	coord, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Role: "coordinator"})
	if err != nil {
		t.Fatalf("create coordinator session: %v", err)
	}
	plain, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID})
	if err != nil {
		t.Fatalf("create plain session: %v", err)
	}

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	// Coordinator session: slot is claimed — a notification must fall into pending,
	// and the cap state must survive (autonomous claim does not reset it).
	slot := rt.coordSlotFor(coord.ID)
	slot.mu.Lock()
	slot.turns = 7
	slot.mu.Unlock()
	release := rt.claimTurnSlotIfCoordinator(ctx, coord.ID)
	rt.enqueueCoordinatorTurn(coord.ID)
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if turns != 0 {
		mu.Unlock()
		t.Fatalf("auto turn ran while autonomous turn held the slot: turns = %d", turns)
	}
	mu.Unlock()
	slot.mu.Lock()
	if slot.turns != 7 {
		slot.mu.Unlock()
		t.Fatalf("autonomous claim must not reset the auto-turn cap, turns = %d", slot.turns)
	}
	slot.mu.Unlock()
	release()

	// Non-coordinator session: release is a no-op and nothing blocks.
	releasePlain := rt.claimTurnSlotIfCoordinator(ctx, plain.ID)
	releasePlain()
}

// TestSpawnWorkerRespectsWorkerCap verifies the per-coordinator worker cap refuses
// a spawn once the active-worker count is at the limit.
func TestSpawnWorkerRespectsWorkerCap(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.tun.SetCoordinatorLimits(2, 0)
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Simulate two workers already active under this coordinator.
	slot := rt.coordSlotFor("COORD")
	slot.workers.Add(2)

	if _, err := rt.SpawnWorker(ctx, "COORD", "W", "task", "", ""); err == nil {
		t.Fatal("expected worker-limit error when the cap is already reached")
	}
}

// TestSpawnWorkerMaterializesProfile verifies a profile target (explore) is
// materialized once into a reusable "worker:explore" agent cloned from the base
// coordinator agent, and reused on the second spawn.
func TestSpawnWorkerMaterializesProfile(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	base, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Coord", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create base agent: %v", err)
	}

	r1, err := rt.SpawnWorker(ctx, "COORD", "explore", "map the code", "", base.ID)
	if err != nil {
		t.Fatalf("spawn worker (explore): %v", err)
	}
	s1, _ := rt.db.GetSession(ctx, r1.SessionID)
	wa, err := rt.resolveAgent(ctx, "worker:explore")
	if err != nil {
		t.Fatalf("profile worker agent not materialized: %v", err)
	}
	if s1.AgentID != wa.ID {
		t.Errorf("worker session agent = %q, want materialized %q", s1.AgentID, wa.ID)
	}
	if wa.Provider != "anthropic" {
		t.Errorf("materialized worker should clone provider, got %q", wa.Provider)
	}

	// Second spawn reuses the same agent (no duplicate).
	if _, err := rt.SpawnWorker(ctx, "COORD", "explore", "again", "", base.ID); err != nil {
		t.Fatalf("second spawn: %v", err)
	}
	agents, _ := rt.db.ListAgents(ctx)
	n := 0
	for _, a := range agents {
		if a.Name == "worker:explore" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected exactly one worker:explore agent, got %d", n)
	}
}

// TestSpawnWorkerSetsCoordinatorLink confirms a worker spawn creates a worker-kind
// session linked back to its coordinator.
func TestSpawnWorkerSetsCoordinatorLink(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	res, err := rt.SpawnWorker(ctx, "COORD", "W", "do it", "", "")
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	sess, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Kind != "worker" {
		t.Errorf("kind = %q, want worker", sess.Kind)
	}
	if sess.Role != "worker" {
		t.Errorf("role = %q, want worker", sess.Role)
	}
	if sess.CoordinatorSessionID != "COORD" {
		t.Errorf("coordinatorSessionID = %q, want COORD", sess.CoordinatorSessionID)
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

// TestIdleReconcileSweepRunsFinalTurn: when every worker has finished (workers==0)
// under a coordinator that actually spawned workers, the drain loop must run ONE
// extra authoritative reconcile turn — the liveness backstop against a coalesced
// notification the model overlooked. It must be one-shot per all-idle transition
// and re-arm on the next notification.
func TestIdleReconcileSweepRunsFinalTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	slot := rt.coordSlotFor("COORD")
	slot.markHadWorkers() // simulate a coordinator that has spawned worker(s), now all done

	// One notification: the processing turn (1) plus the idle-reconcile turn (2).
	rt.enqueueCoordinatorTurn("COORD")
	waitTurns(t, &mu, &turns, 2, "process + reconcile")

	// One-shot: no further idle turn may appear without a new notification.
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	got := turns
	mu.Unlock()
	if got != 2 {
		t.Fatalf("idle sweep must be one-shot; got %d turns", got)
	}

	// A new notification re-arms the sweep → process (3) + reconcile (4).
	rt.enqueueCoordinatorTurn("COORD")
	waitTurns(t, &mu, &turns, 4, "re-armed process + reconcile")
}

// TestIdleReconcileSkippedWithoutWorkers: a coordinator that never spawned a worker
// must NOT get an idle-reconcile turn (nothing to reconcile) — a single notification
// yields exactly one turn.
func TestIdleReconcileSkippedWithoutWorkers(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	rt.enqueueCoordinatorTurn("COORD")
	waitTurns(t, &mu, &turns, 1, "single turn, no reconcile")
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	got := turns
	mu.Unlock()
	if got != 1 {
		t.Fatalf("no-worker coordinator must not reconcile; got %d turns", got)
	}
}
