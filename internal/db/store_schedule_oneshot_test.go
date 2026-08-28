package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestConsumeOneShotSchedule_AtMostOnce pins the store-level guarantee the wake
// scheduler relies on: only the first claimant of a one-shot row wins, the loser
// gets ok=false with NO error (skip, do not fail), and the consumed row leaves
// the enabled set — which is what stops an overdue wake from being handed out on
// every tick. Persisted state is re-read from disk so a restart sees it too.
func TestConsumeOneShotSchedule_AtMostOnce(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()

	sc, err := d.CreateSchedule(ctx, Schedule{
		AgentID: "AG1", Prompt: "Devam et", SessionID: "SES1",
		OneShot: true, FireAt: time.Now().Add(-time.Minute).Unix(), Enabled: true,
	})
	if err != nil {
		t.Fatalf("create one-shot: %v", err)
	}

	claimed, ok, err := d.ConsumeOneShotSchedule(ctx, sc.ID)
	if err != nil || !ok {
		t.Fatalf("first claim must win: ok=%v err=%v", ok, err)
	}
	if claimed.Enabled || claimed.LastRunAt == 0 {
		t.Errorf("claimed row must be disabled and stamped, got %+v", claimed)
	}

	if _, ok, err := d.ConsumeOneShotSchedule(ctx, sc.ID); err != nil || ok {
		t.Fatalf("second claim must lose without an error: ok=%v err=%v", ok, err)
	}

	enabled, err := d.ListEnabledSchedules(ctx)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	for _, e := range enabled {
		if e.ID == sc.ID {
			t.Fatalf("consumed wake still selectable by the scheduler: %+v", e)
		}
	}

	// Reopened store (process restart): the claim must still hold.
	d2, err := Open(d.root)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, ok, err := d2.ConsumeOneShotSchedule(ctx, sc.ID); err != nil || ok {
		t.Fatalf("claim must survive a restart: ok=%v err=%v", ok, err)
	}
}

// TestConsumeOneShotSchedule_RejectsCron guards that a recurring schedule can
// never be burned by the one-shot path — it must error, not silently succeed.
func TestConsumeOneShotSchedule_RejectsCron(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()

	sc, err := d.CreateSchedule(ctx, Schedule{
		AgentID: "AG1", Prompt: "x", CronExpr: "0 9 * * *", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cron schedule: %v", err)
	}
	if _, ok, err := d.ConsumeOneShotSchedule(ctx, sc.ID); ok || !errors.Is(err, ErrNotOneShot) {
		t.Fatalf("cron row must be rejected with ErrNotOneShot: ok=%v err=%v", ok, err)
	}

	if _, ok, err := d.ConsumeOneShotSchedule(ctx, "SCH-missing"); ok || !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id must be ErrNotFound: ok=%v err=%v", ok, err)
	}
}
