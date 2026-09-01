package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestChatTurnStalledStreamIsReclaimed(t *testing.T) {
	const idle = 50 * time.Millisecond
	ctx, stop := WithChatActivityTimeout(context.Background(), 0, idle)
	defer stop()
	tracker := ActivityTrackerFrom(ctx)
	tracker.ObserveStep(TurnStep{Kind: StepDelta, Text: "first"})
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), ErrTurnIdleTimeout) {
			t.Fatalf("cause = %v", context.Cause(ctx))
		}
	case <-time.After(time.Second):
		t.Fatal("stalled stream was not reclaimed")
	}
}

func TestHeartbeatLikeNoiseDoesNotKeepChatAlive(t *testing.T) {
	const idle = 45 * time.Millisecond
	ctx, stop := WithChatActivityTimeout(context.Background(), 0, idle)
	defer stop()
	tracker := ActivityTrackerFrom(ctx)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if tracker.Snapshot().Sequence != 0 {
				t.Fatalf("noise advanced tracker: %+v", tracker.Snapshot())
			}
			return
		case <-ticker.C:
			tracker.ObserveStep(TurnStep{Kind: StepDelta})
			tracker.ObserveStep(TurnStep{Kind: StepTombstone, Ref: "heartbeat"})
		case <-time.After(time.Second):
			t.Fatal("noise kept chat alive")
		}
	}
}

func TestChatActivityTimeoutHardCeilingStillAvailableForExplicitFailsafe(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 35*time.Millisecond, time.Second)
	defer stop()
	<-ctx.Done()
	if !errors.Is(context.Cause(ctx), ErrTurnHardTimeout) {
		t.Fatalf("cause = %v", context.Cause(ctx))
	}
}
