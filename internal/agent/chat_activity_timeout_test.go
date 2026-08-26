package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
