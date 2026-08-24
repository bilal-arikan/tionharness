package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestCoordinatorStallGuardExemptsRecentWorkerNote reproduces WS24/SES34: a
// worker result lands, the coordinator finishes a digest turn without a
// coordination tool call, and no workers remain. That is not a phantom spawn.
func TestCoordinatorStallGuardExemptsRecentWorkerNote(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	coord := newTestCoordinator(t, rt, 0)
	agent := db.Agent{ID: "A", Name: "Coord", Provider: "missing", Model: "m"}

	if _, err := rt.recordInjectedUserNote(ctx, coord, "worker-note", "SES80 completed"); err != nil {
		t.Fatalf("record worker note: %v", err)
	}
	if _, err := rt.db.AddMessage(ctx, db.Message{
		SessionID: coord, AgentID: agent.ID, Role: "assistant",
		Text: "I have digested the result; opening the next round.",
	}); err != nil {
		t.Fatalf("record coordinator turn: %v", err)
	}

	if !rt.hasRecentWorkerNoteInbound(coord, time.Now()) {
		t.Fatal("expected recent worker-note exemption")
	}
	rt.guardCoordinatorStall(coord, agent.ID, agent, "opening the next round", nil)

	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.spawnHallucStreak != 0 || slot.pending || slot.stallHalted {
		t.Fatalf("guard fired after fresh worker note: streak=%d pending=%v halted=%v",
			slot.spawnHallucStreak, slot.pending, slot.stallHalted)
	}
	sess, err := rt.db.GetSession(ctx, coord)
	if err != nil {
		t.Fatalf("get coordinator: %v", err)
	}
	if sess.StallNudges != 0 {
		t.Fatalf("StallNudges = %d, want 0", sess.StallNudges)
	}
}

// TestCoordinatorStallHaltGatesWorkerNoteWake proves the halt message is now true:
// a worker note is persisted but cannot start an automatic coordinator turn.
func TestCoordinatorStallHaltGatesWorkerNoteWake(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)
	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	slot.stallHalted = true
	slot.mu.Unlock()

	rt.NotifyCoordinator(coord, "worker finished")
	time.Sleep(20 * time.Millisecond)

	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.driving || slot.pending {
		t.Fatalf("halted worker-note wake started a turn: driving=%v pending=%v", slot.driving, slot.pending)
	}
}
