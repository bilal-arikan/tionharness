package db

import (
	"context"
	"testing"
)

// TestClaimWaitingFlowRun_CASGuardsDoubleResume verifies the waiting→running CAS:
// a run must be marked waiting before it can be claimed, exactly one claim wins
// (concurrent input from multiple windows), and a claimed/finished run rejects
// further claims.
func TestClaimWaitingFlowRun_CASGuardsDoubleResume(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	run, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW1", Input: "hi"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.Status != FlowRunning {
		t.Fatalf("new run should be running, got %q", run.Status)
	}

	// A running run cannot be claimed (only waiting runs accept input).
	if _, err := d.ClaimWaitingFlowRun(ctx, run.ID); err == nil {
		t.Fatal("claiming a running (non-waiting) run should fail")
	}

	// Suspend it: state + waiting status in one atomic step.
	if err := d.MarkFlowRunWaiting(ctx, run.ID, `{"current":"w","waitingAt":"w"}`); err != nil {
		t.Fatalf("mark waiting: %v", err)
	}
	got, _ := d.GetFlowRun(ctx, run.ID)
	if got.Status != FlowWaiting {
		t.Fatalf("run should be waiting, got %q", got.Status)
	}

	// First claim wins → running.
	claimed, err := d.ClaimWaitingFlowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("first claim should win: %v", err)
	}
	if claimed.Status != FlowRunning {
		t.Fatalf("claimed run should be running, got %q", claimed.Status)
	}

	// Second concurrent claim loses (already running) — the double-resume guard.
	if _, err := d.ClaimWaitingFlowRun(ctx, run.ID); err == nil {
		t.Fatal("second claim must fail (double-resume guard)")
	}

	// Waiting runs are excluded from the boot-resume list (no orphan revival).
	if err := d.MarkFlowRunWaiting(ctx, run.ID, "{}"); err != nil {
		t.Fatalf("re-mark waiting: %v", err)
	}
	running, _ := d.ListRunningFlowRuns(ctx)
	for _, r := range running {
		if r.ID == run.ID {
			t.Fatal("a waiting run must not appear in ListRunningFlowRuns (boot would orphan-revive it)")
		}
	}
}
