package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
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

// TestFireWake_DeliversAtMostOnce is the regression guard for the wake re-fire
// loop: an overdue one-shot row stayed enabled for the whole delivery, so every
// scheduler rebuild re-armed it and the same wake was delivered again and again.
// Two ticks on the same schedule must produce exactly one delivery.
func TestFireWake_DeliversAtMostOnce(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()

	// Anthropic agent with no API key → deliverWake fails, which must still consume
	// the wake (an attempt that started counts as spent).
	agent, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Bekçi", Provider: "anthropic", Model: "m"})
	sess, _ := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat"})
	sc, err := rt.db.CreateSchedule(ctx, db.Schedule{
		AgentID: agent.ID, Prompt: "Devam et", SessionID: sess.ID,
		OneShot: true, FireAt: time.Now().Add(-10 * time.Second).Unix(), Enabled: true,
	})
	if err != nil {
		t.Fatalf("create one-shot: %v", err)
	}

	sched.fireWake(sc.ID)
	msgs, err := rt.db.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected wake prompt + error reply after the first tick, got %d: %+v", len(msgs), msgs)
	}

	// Failed delivery: the row survives, disabled and stamped, with the reason kept.
	got, err := rt.db.GetSchedule(ctx, sc.ID)
	if err != nil {
		t.Fatalf("get schedule: %v", err)
	}
	if got.Enabled {
		t.Error("a fired wake must be disabled, not left enabled for the next tick")
	}
	if got.LastRunAt == 0 {
		t.Error("a fired wake must stamp lastRunAt")
	}
	if got.LastDeliveryStatus != "failure" || got.LastDeliveryError == "" {
		t.Errorf("failed delivery must be recorded, got status=%q error=%q",
			got.LastDeliveryStatus, got.LastDeliveryError)
	}

	// Second tick (a duplicate timer armed by a rebuild): no second delivery.
	sched.fireWake(sc.ID)
	msgs, err = rt.db.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("wake re-delivered on the second tick: %d messages, want 2: %+v", len(msgs), msgs)
	}

	// A rebuild must not re-arm the spent row either.
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sched.Stop()
	sched.mu.Lock()
	timers := len(sched.wakeTimers)
	sched.mu.Unlock()
	if timers != 0 {
		t.Errorf("spent wake re-armed on reload: %d timers", timers)
	}
}

// TestScheduler_RetiresStaleWake verifies a wake whose fire time is long past
// (e.g. the app was down) is consumed undelivered instead of being queued again.
func TestScheduler_RetiresStaleWake(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()

	agent, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Bekçi", Provider: "anthropic", Model: "m"})
	sess, _ := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat"})
	sc, err := rt.db.CreateSchedule(ctx, db.Schedule{
		AgentID: agent.ID, Prompt: "Devam et", SessionID: sess.ID,
		OneShot: true, FireAt: time.Now().Add(-2 * staleWakeAge).Unix(), Enabled: true,
	})
	if err != nil {
		t.Fatalf("create one-shot: %v", err)
	}

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sched.Stop()

	sched.mu.Lock()
	timers := len(sched.wakeTimers)
	sched.mu.Unlock()
	if timers != 0 {
		t.Errorf("stale wake armed anyway: %d timers", timers)
	}
	got, err := rt.db.GetSchedule(ctx, sc.ID)
	if err != nil {
		t.Fatalf("get schedule: %v", err)
	}
	if got.Enabled || got.LastRunAt == 0 || got.LastDeliveryStatus != "expired" {
		t.Errorf("stale wake not retired: %+v", got)
	}
	msgs, _ := rt.db.ListMessages(ctx, sess.ID)
	if len(msgs) != 0 {
		t.Errorf("stale wake must not be delivered, got %d messages", len(msgs))
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
