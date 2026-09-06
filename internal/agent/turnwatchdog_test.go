package agent

import (
	"testing"
	"time"
)

func TestTurnIdleWatchdogDefault(t *testing.T) {
	tun := NewTunables()
	if got, want := tun.TurnIdleWatchdog(), DefaultTurnIdleWatchdogMinutes*time.Minute; got != want {
		t.Fatalf("default idle = %v, want %v", got, want)
	}
}

func TestTurnIdleWatchdogIndependentFromLegacyHardLimit(t *testing.T) {
	tun := NewTunables()
	tun.SetTurnIdleWatchdogMinutes(90)
	if got := tun.TurnIdleWatchdog(); got != 90*time.Minute {
		t.Fatalf("idle was capped by deprecated hard limit: %v", got)
	}
}
