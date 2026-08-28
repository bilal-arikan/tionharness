package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestDeliverPrompt_SpawnModeOpensFreshSession locks the "spawn" session mode:
// every fire must open a NEW independent session instead of appending another turn
// to the agent's shared "schedule" thread, which stays empty.
func TestDeliverPrompt_SpawnModeOpensFreshSession(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Özetçi", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sc := db.Schedule{
		AgentID:     agent.ID,
		Name:        "Günlük özet",
		Prompt:      "Günlük özet ver",
		CronExpr:    "0 * * * *",
		Enabled:     true,
		SessionMode: db.ScheduleSessionModeSpawn,
	}

	// The spawned turn runs in the background (and fails on the unconfigured
	// provider); deliverPrompt returns as soon as the session exists.
	first, err := sched.deliverPrompt(ctx, sc)
	if err != nil {
		t.Fatalf("first spawn fire: %v", err)
	}
	second, err := sched.deliverPrompt(ctx, sc)
	if err != nil {
		t.Fatalf("second spawn fire: %v", err)
	}
	if first == "" || second == "" {
		t.Fatalf("expected a session id per fire, got %q and %q", first, second)
	}
	if first == second {
		t.Fatalf("both fires reused session %q; spawn mode must open a fresh one each time", first)
	}

	session, err := rt.db.GetSession(ctx, first)
	if err != nil {
		t.Fatalf("get spawned session: %v", err)
	}
	if session.Kind != "schedule-run" {
		t.Errorf("spawned session kind = %q, want %q", session.Kind, "schedule-run")
	}
	if session.AgentID != agent.ID {
		t.Errorf("spawned session agent = %q, want %q", session.AgentID, agent.ID)
	}

	// The shared schedule-kind thread must be untouched by a spawn-mode fire.
	shared, err := rt.db.GetOrCreateKindSession(ctx, agent.ID, "schedule", "⏰ Schedule")
	if err != nil {
		t.Fatalf("get schedule session: %v", err)
	}
	msgs, err := rt.db.ListMessages(ctx, shared.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("spawn mode wrote %d messages into the shared schedule session: %+v", len(msgs), msgs)
	}
}

// TestScheduleEffectiveSessionMode locks the default: an empty mode (every row
// written before the field existed) keeps reusing the schedule thread.
func TestScheduleEffectiveSessionMode(t *testing.T) {
	cases := []struct{ stored, want string }{
		{"", db.ScheduleSessionModeReuse},
		{db.ScheduleSessionModeReuse, db.ScheduleSessionModeReuse},
		{db.ScheduleSessionModeSpawn, db.ScheduleSessionModeSpawn},
	}
	for _, c := range cases {
		if got := (db.Schedule{SessionMode: c.stored}).EffectiveSessionMode(); got != c.want {
			t.Errorf("EffectiveSessionMode(%q) = %q, want %q", c.stored, got, c.want)
		}
	}
}
