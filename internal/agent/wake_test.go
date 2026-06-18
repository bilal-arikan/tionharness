package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal/swarmgo/internal/db"
)

// TestScheduleWake_ArmsOneShot verifies ScheduleWake persists a one-shot schedule
// tied to the originating session (not a cron routine) and clamps the delay.
func TestScheduleWake_ArmsOneShot(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	// Reloader is a no-op here (no scheduler wired) — arming is exercised separately.
	rt.SetScheduleReloader(func(context.Context) error { return nil })
	ctx := context.Background()

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Bekçi", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// A below-minimum delay must clamp up to MinWakeDelaySec, not fire instantly.
	if _, err := rt.ScheduleWake(ctx, sess.ID, agent.ID, "Devam et", "bekliyorum", 1); err != nil {
		t.Fatalf("schedule wake: %v", err)
	}

	all, _ := rt.db.ListSchedules(ctx)
	if len(all) != 1 {
		t.Fatalf("expected 1 one-shot schedule, got %d", len(all))
	}
	sc := all[0]
	if !sc.OneShot || sc.SessionID != sess.ID || sc.CronExpr != "" {
		t.Errorf("expected one-shot tied to session with empty cron, got %+v", sc)
	}
	if min := time.Now().Add(time.Duration(MinWakeDelaySec-1) * time.Second).Unix(); sc.FireAt < min {
		t.Errorf("FireAt %d not clamped to >= ~%ds in the future", sc.FireAt, MinWakeDelaySec)
	}

	// Empty session id is rejected (wake only makes sense inside a chat turn).
	if _, err := rt.ScheduleWake(ctx, "", agent.ID, "x", "", 10); err == nil {
		t.Error("expected error for empty session id")
	}
}

// TestDeliverWake_TargetsOriginalSession verifies a wake delivers into its
// originating chat session (not a separate schedule thread) and records the
// failure inline when the provider is unconfigured — so the conversation visibly
// continues instead of dangling.
func TestDeliverWake_TargetsOriginalSession(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Bekçi", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sc := db.Schedule{AgentID: agent.ID, Prompt: "Tarama sonuçlarını kontrol et", SessionID: sess.ID, OneShot: true}

	if err := sched.deliverWake(ctx, sc); err == nil {
		t.Fatal("expected deliverWake to fail with unconfigured provider")
	}

	msgs, err := rt.db.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected wake prompt + error reply in the ORIGINAL session, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Text != "Tarama sonuçlarını kontrol et" {
		t.Errorf("first message should be the wake prompt, got %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].AgentID != agent.ID {
		t.Errorf("reply should be an assistant turn stamped with the agent, got %+v", msgs[1])
	}
	if !strings.Contains(msgs[1].Text, "çalıştırılamadı") {
		t.Errorf("reply must explain the failure, got %q", msgs[1].Text)
	}
}

// TestScheduler_StartSkipsOneShotCron guards that a one-shot row never reaches the
// cron table (its empty CronExpr would error): Start must arm it as a timer and
// leave the cron entries empty.
func TestScheduler_StartSkipsOneShotCron(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()

	agent, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Bekçi", Provider: "anthropic", Model: "m"})
	sess, _ := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat"})
	if _, err := rt.db.CreateSchedule(ctx, db.Schedule{
		AgentID: agent.ID, Prompt: "x", SessionID: sess.ID,
		OneShot: true, FireAt: time.Now().Add(time.Hour).Unix(), Enabled: true,
	}); err != nil {
		t.Fatalf("create one-shot: %v", err)
	}

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("scheduler start with a one-shot row failed: %v", err)
	}
	sched.mu.Lock()
	cronEntries, wakeTimers := len(sched.entries), len(sched.wakeTimers)
	sched.mu.Unlock()
	if cronEntries != 0 {
		t.Errorf("one-shot must not enter the cron table, got %d cron entries", cronEntries)
	}
	if wakeTimers != 1 {
		t.Errorf("expected 1 armed wake timer, got %d", wakeTimers)
	}
	sched.Stop()
}
