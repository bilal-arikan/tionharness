package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// stallTierOutcome replays the cumulative-count decision the turn-end guard makes
// AFTER the judge has already confirmed a stall: count this nudge, then halt if the
// persisted tally reached the threshold. The judge itself needs a live provider, so
// the tests below drive everything downstream of its verdict — which is exactly the
// tier under test — rather than stubbing a model call.
func stallTierOutcome(t *testing.T, rt *Runtime, coord string, slot *coordSlot) (total int, halted bool) {
	t.Helper()
	agent := db.Agent{ID: "A", Name: "Coord"}
	total = rt.injectStallNudge(coord, agent.ID, slot, true /* re-arm this batch */)
	if limit := rt.tun.CoordinatorStallHaltTotal(); limit > 0 && total >= limit {
		rt.escalateCoordinatorStallHalt(coord, agent.ID, agent, slot, "cumulative stall threshold reached")
	}
	return total, rt.CoordinatorStallHalted(coord)
}

// TestCumulativeStallHaltThreshold is the tier's table: below the threshold the
// coordinator keeps its turn (nudged and re-armed, NOT halted); at the threshold it
// is halted. The nudge cap is raised out of the way so the only thing that can halt
// here is the cumulative counter — otherwise the consecutive-streak tier would fire
// first and the test would pass for the wrong reason.
func TestCumulativeStallHaltThreshold(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	tun.SetCoordinatorStallGuard(true, 0, 99)
	tun.SetCoordinatorStallHaltTotal(3)

	coord := newTestCoordinator(t, rt, 0)
	slot := rt.coordSlotFor(coord)

	for _, tc := range []struct {
		name       string
		wantTotal  int
		wantHalted bool
		wantArmed  bool // the turn continues: one more turn is re-armed to act on the note
	}{
		{"first stall: nudged, turn continues", 1, false, true},
		{"second stall: still under threshold", 2, false, true},
		{"third stall: threshold reached, halted", 3, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			total, halted := stallTierOutcome(t, rt, coord, slot)
			if total != tc.wantTotal {
				t.Fatalf("persisted tally = %d, want %d", total, tc.wantTotal)
			}
			if halted != tc.wantHalted {
				t.Fatalf("halted = %v, want %v (tally %d/%d)", halted, tc.wantHalted, total, tun.CoordinatorStallHaltTotal())
			}
			// The halt must actually stop the drain loop re-arming; below it, the
			// coordinator must still get its next turn. A tier that halted but left
			// pending set would keep auto-turning the wedged coordinator anyway.
			slot.mu.Lock()
			pending := slot.pending
			slot.mu.Unlock()
			if pending != tc.wantArmed {
				t.Fatalf("slot.pending = %v, want %v", pending, tc.wantArmed)
			}
			// The tally is read back from the store, not just the return value: this
			// tier's whole reason to exist is that it survives a process restart.
			got, err := rt.db.GetSession(ctx, coord)
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			if got.StallNudges != tc.wantTotal {
				t.Fatalf("persisted StallNudges = %d, want %d", got.StallNudges, tc.wantTotal)
			}
		})
	}
}

// TestCleanCoordinatorTurnResetsCumulativeTally verifies the reset half of the tier:
// a turn that actually calls a coordination tool clears BOTH counters, so a
// coordinator that recovers gets a full budget again instead of carrying a lifetime
// total that eventually halts a healthy session.
func TestCleanCoordinatorTurnResetsCumulativeTally(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	tun.SetCoordinatorStallGuard(true, 0, 99)
	tun.SetCoordinatorStallHaltTotal(3)

	coord := newTestCoordinator(t, rt, 0)
	slot := rt.coordSlotFor(coord)
	agent := db.Agent{ID: "A", Name: "Coord"}

	// Two stalls: one short of the threshold.
	stallTierOutcome(t, rt, coord, slot)
	stallTierOutcome(t, rt, coord, slot)
	if got, _ := rt.db.GetSession(ctx, coord); got.StallNudges != 2 {
		t.Fatalf("setup: StallNudges = %d, want 2", got.StallNudges)
	}

	// A genuine coordination turn. The guard's first branch takes it before any judge
	// call, so this runs the real production path end to end.
	steps := []TurnStep{{Kind: StepTool, Tool: "spawn_worker"}}
	rt.guardCoordinatorStall(coord, agent.ID, agent, "spawning a worker", steps)

	got, err := rt.db.GetSession(ctx, coord)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.StallNudges != 0 {
		t.Fatalf("a clean coordination turn must clear the persisted tally, got %d", got.StallNudges)
	}
	slot.mu.Lock()
	streak, halted := slot.spawnHallucStreak, slot.stallHalted
	slot.mu.Unlock()
	if streak != 0 || halted {
		t.Fatalf("a clean turn must clear the in-memory streak and halt too; streak=%d halted=%v", streak, halted)
	}

	// The budget really is fresh: two more stalls must still not halt, because the
	// counter restarted from zero rather than resuming at 2.
	stallTierOutcome(t, rt, coord, slot)
	total, halt := stallTierOutcome(t, rt, coord, slot)
	if total != 2 || halt {
		t.Fatalf("after a reset the tier must start over; total=%d halted=%v", total, halt)
	}
}

// TestCumulativeStallTierDisabled verifies 0 turns the tier off entirely: the tally
// still counts (it stays a forensic signal) but no amount of it halts the
// coordinator — only the consecutive nudge budget can, as before this tier existed.
func TestCumulativeStallTierDisabled(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorStallGuard(true, 0, 99)
	tun.SetCoordinatorStallHaltTotal(0)

	coord := newTestCoordinator(t, rt, 0)
	slot := rt.coordSlotFor(coord)

	for i := 0; i < 5; i++ {
		if _, halted := stallTierOutcome(t, rt, coord, slot); halted {
			t.Fatalf("tier disabled (0) must never halt; halted after %d stalls", i+1)
		}
	}
}

// TestResumeClearsCumulativeTally verifies the user-facing "Devam ettir" action gives
// a genuinely fresh budget: leaving the persisted tally at the threshold would
// re-halt the coordinator on its very next stall, making the button look broken.
func TestResumeClearsCumulativeTally(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	tun.SetCoordinatorStallGuard(true, 0, 99)
	tun.SetCoordinatorStallHaltTotal(3)
	rt.coordRunFn = func(string) {} // resume kicks a turn; keep it a no-op

	coord := newTestCoordinator(t, rt, 0)
	slot := rt.coordSlotFor(coord)
	for i := 0; i < 3; i++ {
		stallTierOutcome(t, rt, coord, slot)
	}
	if !rt.CoordinatorStallHalted(coord) {
		t.Fatal("setup: expected the coordinator to be halted at the threshold")
	}

	if err := rt.ResumeCoordinatorFromStall(ctx, coord); err != nil {
		t.Fatalf("resume: %v", err)
	}
	got, err := rt.db.GetSession(ctx, coord)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.StallNudges != 0 {
		t.Fatalf("resume must clear the persisted tally, got %d", got.StallNudges)
	}
	if _, halted := stallTierOutcome(t, rt, coord, slot); halted {
		t.Fatal("after a resume the very next stall must not immediately re-halt")
	}
}
