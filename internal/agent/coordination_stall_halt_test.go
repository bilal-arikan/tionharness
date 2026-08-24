package agent

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestEscalateCoordinatorStallHaltIsOneShot verifies the hard-halt escalation
// (FND-99caeb31): it marks the slot halted, clears any pending re-arm, and is
// idempotent — a second call (turn-end guard AND sweeper can both reach it) must not
// bump the streak or otherwise re-fire, so the user notice is posted only once.
func TestEscalateCoordinatorStallHaltIsOneShot(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)
	agent := db.Agent{ID: "A", Name: "Coord"}

	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	slot.spawnHallucStreak = 2
	slot.pending = true
	slot.mu.Unlock()

	rt.escalateCoordinatorStallHalt(coord, agent.ID, agent, slot, "nudge budget spent")

	slot.mu.Lock()
	halted, pending, streak := slot.stallHalted, slot.pending, slot.spawnHallucStreak
	slot.mu.Unlock()
	if !halted || pending {
		t.Fatalf("escalation must set stallHalted and clear pending; got halted=%v pending=%v", halted, pending)
	}

	// Second call is a no-op: nothing about the slot state changes.
	rt.escalateCoordinatorStallHalt(coord, agent.ID, agent, slot, "nudge budget spent")
	slot.mu.Lock()
	if slot.spawnHallucStreak != streak || !slot.stallHalted {
		t.Fatalf("second escalation must be a one-shot no-op; streak %d->%d halted=%v", streak, slot.spawnHallucStreak, slot.stallHalted)
	}
	slot.mu.Unlock()
}

// TestCoordinatorStallHaltedReflectsState verifies the UI accessor: false for a
// session with no slot, true after the escalation, and false again after a resume —
// the exact transitions the persistent "durduruldu" badge reads.
func TestCoordinatorStallHaltedReflectsState(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)
	rt.coordRunFn = func(string) {} // resume enqueues a turn; keep it a no-op

	if rt.CoordinatorStallHalted("never-coordinated") {
		t.Error("a session with no coord slot must not report halted")
	}
	if rt.CoordinatorStallHalted(coord) {
		t.Error("a fresh coordinator must not report halted")
	}

	slot := rt.coordSlotFor(coord)
	agent := db.Agent{ID: "A", Name: "Coord"}
	rt.escalateCoordinatorStallHalt(coord, agent.ID, agent, slot, "nudge budget spent")
	if !rt.CoordinatorStallHalted(coord) {
		t.Error("expected halted=true after escalation")
	}

	if err := rt.ResumeCoordinatorFromStall(context.Background(), coord); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if rt.CoordinatorStallHalted(coord) {
		t.Error("resume must clear the halt")
	}
	slot.mu.Lock()
	streak := slot.spawnHallucStreak
	slot.mu.Unlock()
	if streak != 0 {
		t.Errorf("resume must reset the nudge streak, got %d", streak)
	}
}

// TestResumeCoordinatorRejectsNonCoordinator verifies the resume action refuses a
// plain session (so the CTA never silently no-ops on the wrong session).
func TestResumeCoordinatorRejectsNonCoordinator(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Plain", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "chat", SourceID: "test:plain"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := rt.ResumeCoordinatorFromStall(ctx, sess.ID); err == nil {
		t.Error("expected an error resuming a non-coordinator session")
	}
}

// TestStallHaltStopsIdleReconcile verifies the drain loop honors the halt: a halted
// coordinator with finished workers must NOT get the idle-reconcile turn it would
// otherwise receive (hadWorkers && no running worker) — auto-turns are stopped.
func TestStallHaltStopsIdleReconcile(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)

	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	slot.hadWorkers = true // without the halt this alone triggers a second (reconcile) turn
	slot.mu.Unlock()

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) {
		mu.Lock()
		turns++
		mu.Unlock()
		// Simulate the turn-end guard escalating to a hard halt on this turn.
		slot.mu.Lock()
		slot.stallHalted = true
		slot.mu.Unlock()
	}

	rt.enqueueCoordinatorTurn(coord)

	// Wait for the drain loop to go idle.
	deadline := time.After(2 * time.Second)
	for {
		slot.mu.Lock()
		driving := slot.driving
		slot.mu.Unlock()
		if !driving {
			break
		}
		select {
		case <-deadline:
			t.Fatal("drain loop never went idle after the halt")
		case <-time.After(5 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if turns != 1 {
		t.Fatalf("halted coordinator must not run the idle-reconcile turn; ran %d turns", turns)
	}
}

// TestToolCallClearsStallHalt verifies a coordinator that recovers (calls a real
// coordination tool) clears the halt flag, so it resumes normal auto-turning.
func TestToolCallClearsStallHalt(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)
	agent := db.Agent{ID: "A", Name: "Coord"}

	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	slot.stallHalted = true
	slot.spawnHallucStreak = 2
	slot.mu.Unlock()

	// A turn that actually called spawn_worker: the guard's first branch resets both
	// the streak and the halt flag without ever reaching the judge.
	steps := []TurnStep{{Kind: StepTool, Tool: "spawn_worker"}}
	rt.guardCoordinatorStall(coord, agent.ID, agent, "spawning a worker", steps)

	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.stallHalted || slot.spawnHallucStreak != 0 {
		t.Fatalf("a real coordination tool call must clear the halt and streak; halted=%v streak=%d",
			slot.stallHalted, slot.spawnHallucStreak)
	}
}
