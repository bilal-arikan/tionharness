package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// seedWorkerNoteTurn puts a coordinator into the exact state both worker-note tests
// need: a freshly delivered worker result, followed by an assistant turn that made no
// coordination tool call. Returns the agent used for the turn.
func seedWorkerNoteTurn(t *testing.T, rt *Runtime, coord, noteText, turnText string) db.Agent {
	t.Helper()
	ctx := context.Background()
	agent := db.Agent{ID: "A", Name: "Coord", Provider: "missing", Model: "m"}
	if _, err := rt.recordInjectedUserNote(ctx, coord, "worker-note", noteText); err != nil {
		t.Fatalf("record worker note: %v", err)
	}
	if _, err := rt.db.AddMessage(ctx, db.Message{
		SessionID: coord, AgentID: agent.ID, Role: "assistant", Text: turnText,
	}); err != nil {
		t.Fatalf("record coordinator turn: %v", err)
	}
	return agent
}

// TestCoordinatorStallGuardJudgesAfterWorkerNote covers WS24/SES34 from the other
// side: a worker result lands, the coordinator finishes a genuine digest turn with no
// coordination tool call, and no workers remain. The judge MUST still run (that turn is
// no longer skipped outright), but on a negative verdict nothing fires.
func TestCoordinatorStallGuardJudgesAfterWorkerNote(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	coord := newTestCoordinator(t, rt, 0)
	agent := seedWorkerNoteTurn(t, rt, coord, "SES80 completed",
		"I have digested the result; opening the next round.")

	judged := false
	rt.stallJudgeFn = func(context.Context, db.Agent, string) (bool, error) {
		judged = true
		return false, nil
	}

	if !rt.hasRecentWorkerNoteInbound(coord, time.Now()) {
		t.Fatal("expected the worker note to count as fresh")
	}
	rt.guardCoordinatorStall(coord, agent.ID, agent, "opening the next round", nil)

	if !judged {
		t.Fatal("a fresh worker note must not skip the judge (WS19/SES427 blind spot)")
	}
	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.spawnHallucStreak != 0 || slot.pending || slot.stallHalted {
		t.Fatalf("guard fired on a negative verdict: streak=%d pending=%v halted=%v",
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

// TestCoordinatorStallGuardNudgesPhantomSpawnAfterWorkerNote is the WS19/SES427
// regression: right after a worker result the coordinator narrated a hand-off
// ("TSK103 handed to an independent validator") with an empty tool-call list. The old
// blanket exemption skipped the judge entirely and the coordinator froze. The
// corrective note must now be injected and the turn re-armed.
func TestCoordinatorStallGuardNudgesPhantomSpawnAfterWorkerNote(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	coord := newTestCoordinator(t, rt, 0)
	agent := seedWorkerNoteTurn(t, rt, coord, "<task-notification>SES426 completed</task-notification>",
		"TSK103 handed to an independent validator: /root/validate_tsk103_retry")
	rt.stallJudgeFn = func(context.Context, db.Agent, string) (bool, error) { return true, nil }

	rt.guardCoordinatorStall(coord, agent.ID, agent,
		"TSK103 handed to an independent validator: /root/validate_tsk103_retry", nil)

	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	streak, pending, halted := slot.spawnHallucStreak, slot.pending, slot.stallHalted
	slot.mu.Unlock()
	if streak != 1 || !pending {
		t.Fatalf("phantom spawn after a worker note must nudge and re-arm; streak=%d pending=%v", streak, pending)
	}
	if halted {
		t.Fatal("a first nudge must never halt")
	}
	msgs, err := rt.db.ListMessages(ctx, coord)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m.Text, "<coordination-guard>") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the corrective coordination-guard note in the transcript")
	}
}

// TestCoordinatorStallGuardNeverHaltsWhileWorkerNoteFresh keeps the WS24/SES34
// protection: repeated phantom spawns while a worker note is fresh keep earning
// nudges, but neither halt tier (consecutive nudge cap, cumulative tally) may stop
// the coordinator's auto-turns.
func TestCoordinatorStallGuardNeverHaltsWhileWorkerNoteFresh(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorStallGuard(true, 0, 1) // nudge cap of 1: the second stall would normally halt
	tun.SetCoordinatorStallHaltTotal(2)

	coord := newTestCoordinator(t, rt, 0)
	agent := seedWorkerNoteTurn(t, rt, coord, "<task-notification>SES426 completed</task-notification>",
		"TSK103 handed to an independent validator")
	rt.stallJudgeFn = func(context.Context, db.Agent, string) (bool, error) { return true, nil }

	for i := 0; i < 4; i++ {
		rt.guardCoordinatorStall(coord, agent.ID, agent, "TSK103 handed to an independent validator", nil)
		if rt.CoordinatorStallHalted(coord) {
			t.Fatalf("halted on stall %d while the worker note was still fresh", i+1)
		}
	}
	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.spawnHallucStreak != 4 || !slot.pending {
		t.Fatalf("every stall must still nudge and re-arm; streak=%d pending=%v", slot.spawnHallucStreak, slot.pending)
	}
}

// TestCoordinationStatusNoteIsNotAWorkerResult keeps defensive handling for legacy
// standalone status messages: they are runtime instructions, not worker results.
func TestCoordinationStatusNoteIsNotAWorkerResult(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)

	if _, err := rt.recordInjectedUserNote(context.Background(), coord, "worker-note", coordinationStatusNote); err != nil {
		t.Fatalf("record standalone status: %v", err)
	}

	if rt.hasRecentWorkerNoteInbound(coord, time.Now()) {
		t.Fatal("a <coordination-status> note must not count as a fresh worker result")
	}
}

func TestPiggybackedCoordinationStatusRemainsWorkerResult(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)
	note := attachCoordinationStatus("<task-notification>SES9 completed</task-notification>")
	if _, err := rt.recordInjectedUserNote(context.Background(), coord, "worker-note", note); err != nil {
		t.Fatalf("record piggybacked result: %v", err)
	}

	if !rt.hasRecentWorkerNoteInbound(coord, time.Now()) {
		t.Fatal("piggybacked final worker result must retain the worker-note grace window")
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
