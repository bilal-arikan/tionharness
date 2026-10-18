package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestSessionTargetedRoutineContinuesItsSession: a cron schedule that carries
// a SessionID delivers each fire into that session as a new turn — the way a
// coordinator wakes itself for a window hours ahead — and keeps the schedule
// (it is recurring, not a consumed one-shot).
func TestSessionTargetedRoutineContinuesItsSession(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	installFlowTestProvider(t, rt, okProvider(func(string) string { return "pencere gözlendi" }))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()
	a := flowTestAgent(t, rt, "PM")
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := rt.db.CreateSchedule(ctx, db.Schedule{AgentID: a.ID, CronExpr: "5 16 18 10 *", Prompt: "16:05 maç penceresi: canlı gözlemi başlat", SessionID: sess.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := sched.run(ctx, sc.ID, "manual"); err != nil {
		t.Fatalf("run: %v", err)
	}
	msgs, _ := rt.db.ListMessages(ctx, sess.ID)
	if len(msgs) < 2 || !strings.Contains(msgs[0].Text, "canlı gözlemi başlat") || msgs[len(msgs)-1].Text != "pencere gözlendi" {
		t.Fatalf("the fire must continue the target session, got %+v", msgs)
	}
	if kept, err := rt.db.GetSchedule(ctx, sc.ID); err != nil || !kept.Enabled || kept.LastDeliveryStatus != "success" {
		t.Fatalf("a recurring routine stays enabled after a delivered fire: %+v %v", kept, err)
	}
}
