package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestChatTurnStalledStreamIsReclaimed is the SES578 case: an interactive turn
// streams a few steps and then its provider goes silent (half-open socket, a CLI
// subprocess that never exits). The inactivity window must cancel it with
// ErrTurnIdleTimeout so the turn finalizes and the session's queue drains, instead
// of hanging forever.
func TestChatTurnStalledStreamIsReclaimed(t *testing.T) {
	const idle = 80 * time.Millisecond
	ctx, stop := WithChatActivityTimeout(context.Background(), 10*time.Second, idle)
	defer stop()

	// Three real steps arrive, then the stream dies. A streaming completion gets NO
	// heartbeat (only its deltas touch), so nothing holds the watchdog off.
	for i := 0; i < 3; i++ {
		time.Sleep(idle / 4)
		TouchActivity(ctx)
	}
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), ErrTurnIdleTimeout) {
			t.Fatalf("cause = %v, want ErrTurnIdleTimeout", context.Cause(ctx))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stalled chat turn was never reclaimed")
	}
}

// TestChatTurnStillEmittingIsNotCancelled is the other half of the contract: a
// long-but-productive turn (a tool loop streaming for far longer than the idle
// window) must run on up to the hard ceiling. Regression guard for making the chat
// idle window short.
func TestChatTurnStillEmittingIsNotCancelled(t *testing.T) {
	const idle = 40 * time.Millisecond
	ctx, stop := WithChatActivityTimeout(context.Background(), 10*time.Second, idle)
	defer stop()

	steps := time.NewTicker(idle / 4)
	defer steps.Stop()
	// Keep emitting for ~10 idle windows.
	deadline := time.After(10 * idle)
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("productive chat turn was cancelled: %v", context.Cause(ctx))
		case <-steps.C:
			TouchActivity(ctx)
		case <-deadline:
			return
		}
	}
}

// TestChatTurnHeartbeatReachesChatPath pins the wiring the chat turn depends on:
// WithChatActivityTimeout must install the heartbeat interval, so a long step-less
// operation (a non-streaming completion, one multi-minute tool call) inside a chat
// turn is held alive by startActivityHeartbeat. The bare WithActivityTimeout does
// not, and using it on the chat path would cut such an operation at the idle window.
func TestChatTurnHeartbeatReachesChatPath(t *testing.T) {
	const idle = 60 * time.Millisecond
	ctx, stop := WithChatActivityTimeout(context.Background(), 10*time.Second, idle)
	defer stop()
	if got := activityIntervalFrom(ctx); got <= 0 || got >= idle {
		t.Fatalf("heartbeat interval = %v, want a positive value below the idle window", got)
	}
	stopHB := startActivityHeartbeat(ctx)
	defer stopHB()
	select {
	case <-ctx.Done():
		t.Fatalf("heartbeat did not keep a step-less chat turn alive: %v", context.Cause(ctx))
	case <-time.After(3 * idle):
	}
}

func TestChatActivityTimeoutHardCeiling(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 40*time.Millisecond, 15*time.Millisecond)
	defer stop()

	untilHard := time.NewTicker(5 * time.Millisecond)
	defer untilHard.Stop()
	for {
		select {
		case <-ctx.Done():
			if !errors.Is(context.Cause(ctx), ErrTurnHardTimeout) {
				t.Fatalf("cause = %v, want hard timeout", context.Cause(ctx))
			}
			return
		case <-untilHard.C:
			TouchActivity(ctx)
		case <-time.After(250 * time.Millisecond):
			t.Fatal("hard timeout did not fire")
		}
	}
}

func TestChatActivityTimeoutIdleResetsOnlyOnTouch(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 250*time.Millisecond, 30*time.Millisecond)
	defer stop()

	time.Sleep(15 * time.Millisecond)
	TouchActivity(ctx)
	select {
	case <-ctx.Done():
		t.Fatalf("idle timeout fired before reset window: %v", context.Cause(ctx))
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), ErrTurnIdleTimeout) {
			t.Fatalf("cause = %v, want idle timeout", context.Cause(ctx))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("idle timeout did not fire")
	}
}
