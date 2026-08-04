package agent

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
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

	rt.escalateCoordinatorStallHalt(coord, agent.ID, agent, slot)

	slot.mu.Lock()
	halted, pending, streak := slot.stallHalted, slot.pending, slot.spawnHallucStreak
	slot.mu.Unlock()
	if !halted || pending {
		t.Fatalf("escalation must set stallHalted and clear pending; got halted=%v pending=%v", halted, pending)
	}

	// Second call is a no-op: nothing about the slot state changes.
	rt.escalateCoordinatorStallHalt(coord, agent.ID, agent, slot)
	slot.mu.Lock()
	if slot.spawnHallucStreak != streak || !slot.stallHalted {
		t.Fatalf("second escalation must be a one-shot no-op; streak %d->%d halted=%v", streak, slot.spawnHallucStreak, slot.stallHalted)
	}
	slot.mu.Unlock()
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
		running := slot.running
		slot.mu.Unlock()
		if !running {
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
