package agent

import (
	"testing"
	"time"
)

func TestTurnWatchdogDefaultDisabled(t *testing.T) {
	tun := NewTunables()
	if got := tun.TurnWatchdog(); got != 0 {
		t.Fatalf("legacy hard watchdog = %v, want disabled", got)
	}
}

func TestTurnWatchdogLegacyValuePreservedWithoutBecomingIdleSource(t *testing.T) {
	tun := NewTunables()
	tun.SetTurnWatchdogMinutes(120)
	tun.SetTurnIdleWatchdogMinutes(20)
	if got := tun.TurnWatchdog(); got != 120*time.Minute {
		t.Fatalf("legacy watchdog = %v", got)
	}
	if got := tun.TurnIdleWatchdog(); got != 20*time.Minute {
		t.Fatalf("semantic idle = %v", got)
	}
}

func TestTurnIdleWatchdogDefault(t *testing.T) {
	tun := NewTunables()
	if got, want := tun.TurnIdleWatchdog(), DefaultTurnIdleWatchdogMinutes*time.Minute; got != want {
		t.Fatalf("default idle = %v, want %v", got, want)
	}
}

func TestTurnIdleWatchdogIndependentFromLegacyHardLimit(t *testing.T) {
	tun := NewTunables()
	tun.SetTurnWatchdogMinutes(5)
	tun.SetTurnIdleWatchdogMinutes(90)
	if got := tun.TurnIdleWatchdog(); got != 90*time.Minute {
		t.Fatalf("idle was capped by deprecated hard limit: %v", got)
	}
}
