package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestGuardCoordinatorStallOnChatTurn covers WS27/SES90: the coordinator was driven by
// a plain USER chat turn (kind="chat"), narrated two worker spawns, and made no tool
// call at all. That path never reached runCoordinatorTurn, so the guard never ran. The
// exported entry point the chat path now calls must catch it exactly like an auto-turn:
// corrective note recorded, nudge streak bumped, one more turn armed.
func TestGuardCoordinatorStallOnChatTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	coord := newTestCoordinator(t, rt, 0)
	agent := db.Agent{ID: "A", Name: "Coord", Provider: "missing", Model: "m"}

	judged := ""
	rt.stallJudgeFn = func(_ context.Context, _ db.Agent, text string) (bool, error) {
		judged = text
		return true, nil
	}

	// A user chat turn: prose claiming delegation, zero tool steps.
	rt.GuardCoordinatorStall(coord, agent.ID, agent, "İki worker başlattım, sonuçları bekliyorum.", nil)

	if judged == "" {
		t.Fatal("chat turn was never judged; the guard did not run")
	}
	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	streak, pending, coordMode, last := slot.spawnHallucStreak, slot.pending, slot.coordinatorMode, slot.lastTurnUnix
	slot.mu.Unlock()
	if streak != 1 || !pending {
		t.Fatalf("expected one nudge and a re-armed turn; streak=%d pending=%v", streak, pending)
	}
	if !coordMode || last == 0 {
		t.Fatalf("guard must stamp the slot for the sweeper; coordinatorMode=%v lastTurnUnix=%d", coordMode, last)
	}

	msgs, err := rt.db.ListMessages(ctx, coord)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	found := false
	for _, m := range msgs {
		if m.Origin == "coordination-guard" && strings.Contains(m.Text, "<coordination-guard>") {
			found = true
		}
	}
	if !found {
		t.Fatal("no corrective note was recorded for the chat-turn phantom spawn")
	}
}

// TestStallCandidateWithoutWorkers pins the relaxed sweeper gate: a coordinator that
// never spawned a worker — the phantom-spawn case by definition — must be a candidate,
// while a slot that is not in coordinator mode keeps needing a real worker history.
func TestStallCandidateWithoutWorkers(t *testing.T) {
	const now = 1_000_000
	const window = 300

	coordSess := &coordSlot{coordinatorMode: true, hadWorkers: false, lastTurnUnix: now - 600}
	if !slotIsStallCandidate(coordSess, false, now, window) {
		t.Error("a coordinator that never spawned a worker must still be a candidate")
	}

	plain := &coordSlot{coordinatorMode: false, hadWorkers: false, lastTurnUnix: now - 600}
	if slotIsStallCandidate(plain, false, now, window) {
		t.Error("a non-coordinator slot with no worker history must not be a candidate")
	}
}
