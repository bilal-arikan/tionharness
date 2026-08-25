package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestScheduleExpired checks the optional end-date guard: 0 never expires, a
// future date is still live, a past date is expired.
func TestScheduleExpired(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		name      string
		expiresAt int64
		want      bool
	}{
		{"no end date", 0, false},
		{"future", now + 3600, false},
		{"past", now - 3600, true},
	}
	for _, c := range cases {
		if got := scheduleExpired(db.Schedule{ExpiresAt: c.expiresAt}); got != c.want {
			t.Errorf("%s: scheduleExpired(%d) = %v, want %v", c.name, c.expiresAt, got, c.want)
		}
	}
}

// TestExpiredScheduleSkippedOnReload verifies an already-expired schedule is
// auto-disabled and not armed when the scheduler rebuilds its cron table.
func TestExpiredScheduleSkippedOnReload(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "X", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sc, err := rt.db.CreateSchedule(ctx, db.Schedule{
		AgentID: agent.ID, Prompt: "p", CronExpr: "0 * * * *",
		Enabled: true, ExpiresAt: time.Now().Unix() - 60,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sched.Stop()

	got, err := rt.db.GetSchedule(ctx, sc.ID)
	if err != nil {
		t.Fatalf("get schedule: %v", err)
	}
	if got.Enabled {
		t.Error("expired schedule should be auto-disabled on reload")
	}
}

// TestDeliverPrompt_RecordsErrorReply verifies that a failed scheduled prompt
// surfaces the failure as an assistant turn inside the schedule session, instead
// of leaving the thread with a lone user prompt and no reply. Regression guard
// for the "Günlük Özet Ver cevap gelmiyor" report: the agent's provider is
// unconfigured (no key), so invokeTraced errors — the user must still see why.
func TestDeliverPrompt_RecordsErrorReply(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()

	// Agent on the anthropic provider with no API key → registry Get fails.
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Özetçi", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sc := db.Schedule{AgentID: agent.ID, Prompt: "Günlük özet ver", CronExpr: "0 * * * *", Enabled: true}

	sessionID, fireErr := sched.deliverPrompt(ctx, sc)
	if fireErr == nil {
		t.Fatal("expected deliverPrompt to fail with unconfigured provider")
	}
	if sessionID == "" {
		t.Fatal("expected a session id even on failure (for deep-linking)")
	}

	msgs, err := rt.db.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected user prompt + error reply (2 messages), got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Text != "Günlük özet ver" {
		t.Errorf("first message should be the user prompt, got %+v", msgs[0])
	}
	reply := msgs[1]
	if reply.Role != "assistant" {
		t.Errorf("reply role = %q, want assistant", reply.Role)
	}
	if reply.AgentID != agent.ID {
		t.Errorf("reply agentId = %q, want %q (avatar/identity must render)", reply.AgentID, agent.ID)
	}
	if !strings.Contains(reply.Text, "çalıştırılamadı") || !strings.Contains(reply.Text, fireErr.Error()) {
		t.Errorf("error reply must contain the failure reason, got %q", reply.Text)
	}
}

// TestRun_LogsFailureAtErrorLevel guards the "error notification shows but the
// logs view is empty" report: a failed scheduled fire must emit an Error-level
// log record carrying the failure reason, not just a notification + inline reply.
func TestRun_LogsFailureAtErrorLevel(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	var cap capturingHandler
	sched := NewScheduler(rt.db, rt, slog.New(&cap))
	ctx := context.Background()

	// Anthropic agent with no API key → invokeTraced fails inside run().
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Özetçi", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sc, err := rt.db.CreateSchedule(ctx, db.Schedule{AgentID: agent.ID, Prompt: "Günlük özet ver", CronExpr: "0 * * * *", Enabled: true})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	if err := sched.run(ctx, sc.ID, "manual"); err == nil {
		t.Fatal("expected run to fail with unconfigured provider")
	}

	var found bool
	for _, r := range cap.records() {
		if r.Level == slog.LevelError && strings.Contains(r.Message, "schedule fire: failed") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an Error-level 'schedule fire: failed' log record, got %+v", cap.records())
	}
}

// TestRun_SkipsWhenAutonomyPaused verifies the workspace autonomy pause stops a
// prompt-backed schedule before it opens/reuses its session or records the
// prompt as a message — the deeper guardedComplete gate only trips after
// deliverPrompt has already grown the transcript, which is too late.
func TestRun_SkipsWhenAutonomyPaused(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Özetçi", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sc, err := rt.db.CreateSchedule(ctx, db.Schedule{AgentID: agent.ID, Prompt: "Günlük özet ver", CronExpr: "0 * * * *", Enabled: true})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	rt.SetPaused(true)
	if err := sched.run(ctx, sc.ID, "manual"); err != ErrAutonomyPaused {
		t.Fatalf("run() error = %v, want %v", err, ErrAutonomyPaused)
	}

	if _, err := rt.db.GetOrCreateKindSession(ctx, agent.ID, "schedule", "⏰ Schedule"); err != nil {
		t.Fatalf("create schedule session for assertion: %v", err)
	}
	session, err := rt.db.GetOrCreateKindSession(ctx, agent.ID, "schedule", "⏰ Schedule")
	if err != nil {
		t.Fatalf("get schedule session: %v", err)
	}
	msgs, err := rt.db.ListMessages(ctx, session.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("paused fire must not write to the schedule session, got %d messages: %+v", len(msgs), msgs)
	}

	gotSchedule, err := rt.db.GetSchedule(ctx, sc.ID)
	if err != nil {
		t.Fatalf("get schedule: %v", err)
	}
	if !gotSchedule.Enabled {
		t.Error("a pause-skip must not disable the schedule (unlike the stuck-guard path)")
	}
	if gotSchedule.LastDeliveryStatus != "failure" || gotSchedule.LastDeliveryError != ErrAutonomyPaused.Error() {
		t.Errorf("delivery record = status=%q error=%q, want failure/%q",
			gotSchedule.LastDeliveryStatus, gotSchedule.LastDeliveryError, ErrAutonomyPaused.Error())
	}
}

// capturingHandler is a minimal slog.Handler that retains every record, so tests
// can assert on what was logged (level + message).
type capturingHandler struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, r)
	return nil
}
func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler      { return h }
func (h *capturingHandler) records() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.recs...)
}

// TestNotifyLine checks the notification-body condenser: first line only, rune
// cap with ellipsis, and no mid-character split on Turkish text.
func TestNotifyLine(t *testing.T) {
	cases := []struct {
		name, in string
		max      int
		want     string
	}{
		{"short passes through", "Günlük özet", 100, "Günlük özet"},
		{"first line only", "hata oluştu\nikinci satır\nüçüncü", 100, "hata oluştu"},
		{"trims surrounding space", "   boşluklu   ", 100, "boşluklu"},
		{"rune cap with ellipsis", "abcdefghij", 5, "abcde…"},
		{"turkish rune cap not byte cap", "ışĞçöü", 3, "ışĞ…"},
	}
	for _, c := range cases {
		if got := notifyLine(c.in, c.max); got != c.want {
			t.Errorf("%s: notifyLine(%q,%d) = %q, want %q", c.name, c.in, c.max, got, c.want)
		}
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
