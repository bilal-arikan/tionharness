package agent

import (
	"context"
	"testing"
	"time"
)

// A bridged run_subagent (CLI turn) has no step sink; its context carries only
// the turn's tracker. The child's steps must reach that tracker, or the parent's
// idle watchdog cuts the parent mid-delegation (SES287 → SES294).
func TestSubagentStepObserverFeedsBridgedTurnTracker(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	tracker := newActivityTracker(ctx, cancel, time.Hour)
	defer tracker.Stop()

	onStep := subagentStepObserver(WithActivityTracker(context.Background(), tracker), nil)
	if onStep == nil {
		t.Fatal("no step observer for a bridged call carrying the turn tracker")
	}
	before := tracker.Snapshot().Sequence
	onStep(TurnStep{Kind: StepText, Text: "child is working"})
	if after := tracker.Snapshot().Sequence; after <= before {
		t.Fatalf("child step did not count as turn progress (sequence %d → %d)", before, after)
	}
}

func TestSubagentStepObserverWithoutSinkOrTrackerIsNil(t *testing.T) {
	if subagentStepObserver(context.Background(), nil) != nil {
		t.Fatal("expected no observer when neither a sink nor a tracker is present")
	}
}
