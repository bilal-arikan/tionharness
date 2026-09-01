package providers

import (
	"context"
	"testing"
	"time"
)

func TestResolveIdleOutputWindowRaisesToCtxFloor(t *testing.T) {
	ctx := WithMinIdleOutputTimeout(context.Background(), 10*time.Minute)
	if got := resolveIdleOutputWindow(ctx, 90*time.Second); got != 10*time.Minute {
		t.Fatalf("window = %s, want 10m0s", got)
	}
}

func TestResolveIdleOutputWindowKeepsLargerGlobal(t *testing.T) {
	ctx := WithMinIdleOutputTimeout(context.Background(), 2*time.Minute)
	if got := resolveIdleOutputWindow(ctx, 30*time.Minute); got != 30*time.Minute {
		t.Fatalf("window = %s, want 30m0s", got)
	}
}

// A watchdog the operator disabled (codexStdoutIdleSec <= 0) must stay disabled:
// the floor gives a slow call more room, it does not re-arm the watchdog.
func TestResolveIdleOutputWindowLeavesDisabledWatchdogOff(t *testing.T) {
	ctx := WithMinIdleOutputTimeout(context.Background(), 10*time.Minute)
	if got := resolveIdleOutputWindow(ctx, 0); got != 0 {
		t.Fatalf("window = %s, want 0s", got)
	}
}

func TestResolveIdleOutputWindowWithoutFloor(t *testing.T) {
	if got := resolveIdleOutputWindow(context.Background(), 90*time.Second); got != 90*time.Second {
		t.Fatalf("window = %s, want 1m30s", got)
	}
}

func TestWithMinIdleOutputTimeoutIgnoresNonPositive(t *testing.T) {
	ctx := WithMinIdleOutputTimeout(context.Background(), 0)
	if got := IdleOutputFloor(ctx); got != 0 {
		t.Fatalf("floor = %s, want 0s", got)
	}
}
