package agent

import (
	"context"
	"testing"
	"time"
)

// TestActivityTimeoutIdleCancels verifies the idle watchdog cancels a turn that
// never touches within the idle window, well before the hard ceiling.
func TestActivityTimeoutIdleCancels(t *testing.T) {
	ctx, stop := withActivityTimeout(context.Background(), 10*time.Second, 60*time.Millisecond)
	defer stop()
	select {
	case <-ctx.Done():
		// cancelled by idle watchdog — good
	case <-time.After(2 * time.Second):
		t.Fatal("idle watchdog did not cancel a silent turn")
	}
}

// TestActivityTouchKeepsAlive verifies that steady activity (touch) keeps the turn
// alive past the idle window — it survives until touching stops.
func TestActivityTouchKeepsAlive(t *testing.T) {
	ctx, stop := withActivityTimeout(context.Background(), 10*time.Second, 120*time.Millisecond)
	defer stop()
	touch := activityTouchFrom(ctx)
	if touch == nil {
		t.Fatal("touch func not installed on ctx")
	}
	// Touch every 30ms for ~360ms (3× the idle window). Throttling caps resets to
	// ~1/s, so the FIRST touch resets the timer to now+120ms and later touches are
	// throttled; keep touching long enough that a fresh reset lands each second.
	deadline := time.After(360 * time.Millisecond)
	tick := time.NewTicker(30 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal("watchdog cancelled a turn that was actively touching")
		case <-tick.C:
			touch()
		case <-deadline:
			return // survived the idle window while touching — good
		}
	}
}

// TestActivityTimeoutHardCeiling verifies the absolute ceiling fires even when the
// turn keeps touching (idle disabled path uses hard only).
func TestActivityTimeoutHardCeiling(t *testing.T) {
	ctx, stop := withActivityTimeout(context.Background(), 60*time.Millisecond, 0)
	defer stop()
	if activityTouchFrom(ctx) != nil {
		t.Fatal("idle disabled (0) must install no touch")
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("hard ceiling did not fire")
	}
}

// TestSpawnIdleTimeoutTunable verifies the tunable default + override.
func TestSpawnIdleTimeoutTunable(t *testing.T) {
	var tun Tunables
	if got := tun.SpawnIdleTimeout(); got != time.Duration(DefaultSpawnIdleTimeoutMinutes)*time.Minute {
		t.Fatalf("default idle = %v, want %v", got, time.Duration(DefaultSpawnIdleTimeoutMinutes)*time.Minute)
	}
	tun.SetSpawnIdleTimeoutMinutes(12)
	if got := tun.SpawnIdleTimeout(); got != 12*time.Minute {
		t.Fatalf("override idle = %v, want 12m", got)
	}
}
