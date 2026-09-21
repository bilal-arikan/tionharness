package agent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// scheduleArchiveFixture wires the session hook exactly as the workspace manager
// does at boot (manager.go), because the archive→disable path hangs off it.
func scheduleArchiveFixture(t *testing.T) (*Runtime, db.Agent) {
	t.Helper()
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.db.SetSessionHook(rt.OnSessionChange)
	agentRow, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Ada"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return rt, agentRow
}

func scheduleEnabled(t *testing.T, rt *Runtime, id string) bool {
	t.Helper()
	sc, err := rt.db.GetSchedule(context.Background(), id)
	if err != nil {
		t.Fatalf("read schedule %s: %v", id, err)
	}
	return sc.Enabled
}

// TestArchivingASessionDisablesItsSchedules: archiving is a STOP gesture. A
// one-shot wake pointed at the session and the agent's recurring reuse-mode
// schedule (which delivers into that agent's shared "schedule" thread) both go
// quiet, while a schedule bound to a different session keeps its timer.
func TestArchivingASessionDisablesItsSchedules(t *testing.T) {
	ctx := context.Background()
	rt, agentRow := scheduleArchiveFixture(t)

	chat, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: agentRow.ID})
	if err != nil {
		t.Fatalf("create chat session: %v", err)
	}
	other, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: agentRow.ID})
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}

	wake, err := rt.db.CreateSchedule(ctx, db.Schedule{
		AgentID: agentRow.ID, Prompt: "devam", Enabled: true,
		OneShot: true, FireAt: 1 << 40, SessionID: chat.ID,
	})
	if err != nil {
		t.Fatalf("create wake: %v", err)
	}
	bystander, err := rt.db.CreateSchedule(ctx, db.Schedule{
		AgentID: agentRow.ID, Prompt: "devam", Enabled: true,
		OneShot: true, FireAt: 1 << 40, SessionID: other.ID,
	})
	if err != nil {
		t.Fatalf("create bystander wake: %v", err)
	}

	if err := rt.db.SetSessionState(ctx, chat.ID, "archived"); err != nil {
		t.Fatalf("archive chat session: %v", err)
	}
	if scheduleEnabled(t, rt, wake.ID) {
		t.Fatal("a wake bound to the archived session must be disabled — otherwise it keeps firing into a session the user stopped")
	}
	if !scheduleEnabled(t, rt, bystander.ID) {
		t.Fatal("a wake bound to a different session must stay enabled")
	}

	// The recurring reuse-mode binding: the agent's shared "schedule" thread.
	schedSess, err := rt.db.GetOrCreateKindSession(ctx, agentRow.ID, "schedule", "⏰ Schedule")
	if err != nil {
		t.Fatalf("open schedule session: %v", err)
	}
	cron, err := rt.db.CreateSchedule(ctx, db.Schedule{
		AgentID: agentRow.ID, CronExpr: "0 * * * *", Prompt: "rapor", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cron schedule: %v", err)
	}
	spawned, err := rt.db.CreateSchedule(ctx, db.Schedule{
		AgentID: agentRow.ID, CronExpr: "0 * * * *", Prompt: "rapor", Enabled: true,
		SessionMode: db.ScheduleSessionModeSpawn,
	})
	if err != nil {
		t.Fatalf("create spawn-mode schedule: %v", err)
	}

	if err := rt.db.SetSessionState(ctx, schedSess.ID, "archived"); err != nil {
		t.Fatalf("archive schedule session: %v", err)
	}
	if scheduleEnabled(t, rt, cron.ID) {
		t.Fatal("a reuse-mode schedule must be disabled when its shared thread is archived")
	}
	if !scheduleEnabled(t, rt, spawned.ID) {
		t.Fatal("a spawn-mode schedule opens a fresh session per fire, so archiving one thread must not disable it")
	}
}

// TestArchivedSessionRefusesAScheduledTurn: the safety net for a fire already in
// flight (or one that escaped the disable sweep). A wake turn on an archived
// session is refused before it can touch the queue — and the session is NOT
// revived, because reviving on a wake would restart the very loop archiving
// breaks. Every other turn kind keeps passing through untouched.
func TestArchivedSessionRefusesAScheduledTurn(t *testing.T) {
	ctx := context.Background()
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	id := archivedFixture(t, rt)

	release, err := rt.claimSessionTurnSlotCtx(ctx, id, turnqueue.KindWake, "uyandırma")
	if !errors.Is(err, ErrSessionArchived) {
		t.Fatalf("wake claim on an archived session = %v, want ErrSessionArchived", err)
	}
	release() // must be a safe no-op even though nothing was claimed
	if rt.sessionTurnBusy(id) {
		t.Fatal("a refused wake must not hold the session's turn slot")
	}
	state, tags := sessionTags(t, rt, id)
	if state != "archived" {
		t.Fatalf("state = %q, want the session to stay archived — a wake must not resurrect it", state)
	}
	if !containsTag(tags, TagArchived) {
		t.Fatalf("the %q tag must survive a refused wake, tags = %v", TagArchived, tags)
	}

	// A live session is unaffected: the gate reads state, nothing else.
	live, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat"})
	if err != nil {
		t.Fatalf("create live session: %v", err)
	}
	liveRelease, err := rt.claimSessionTurnSlotCtx(ctx, live.ID, turnqueue.KindWake, "uyandırma")
	if err != nil {
		t.Fatalf("wake claim on a live session = %v, want it to proceed", err)
	}
	liveRelease()

	// Other kinds on the archived session still pass (and a user turn reactivates
	// it, per sessionreactivate.go) — the gate is scoped to KindWake alone.
	archived := archivedFixture(t, rt)
	coordRelease, err := rt.claimSessionTurnSlotCtx(ctx, archived, turnqueue.KindCoordinator, "worker bildirimi")
	if err != nil {
		t.Fatalf("coordinator claim on an archived session = %v, want it to proceed", err)
	}
	coordRelease()
}
