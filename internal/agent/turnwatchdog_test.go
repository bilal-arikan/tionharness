package agent

import (
	"testing"
	"time"
)

// TestTurnWatchdogDefault verifies an unconfigured runtime gets the built-in
// ceiling rather than a zero duration (which would cancel every turn instantly).
func TestTurnWatchdogDefault(t *testing.T) {
	tun := NewTunables()
	if got, want := tun.TurnWatchdog(), DefaultTurnWatchdogMinutes*time.Minute; got != want {
		t.Fatalf("default watchdog = %v, want %v", got, want)
	}
}

// TestTurnWatchdogNeverBelowTurnCeilings is the SES76 regression: the queue
// watchdog was a fixed 20 minutes while spawn turns were configured for 120, so a
// healthy long turn was force-cancelled by the very net meant to catch wedges. The
// watchdog must never be tighter than a deadline the other knobs still grant.
func TestTurnWatchdogNeverBelowTurnCeilings(t *testing.T) {
	tun := NewTunables()
	tun.SetTurnWatchdogMinutes(20)
	tun.SetSpawnTimeoutMinutes(120)
	if got, want := tun.TurnWatchdog(), 120*time.Minute; got != want {
		t.Fatalf("watchdog under spawn ceiling = %v, want %v", got, want)
	}

	tun.SetSpawnTimeoutMinutes(0) // back to the spawn default
	tun.SetScheduleTimeoutMinutes(300)
	if got, want := tun.TurnWatchdog(), 300*time.Minute; got != want {
		t.Fatalf("watchdog under schedule ceiling = %v, want %v", got, want)
	}
}

// TestTurnIdleWatchdogDefault verifies the inactivity window falls back to the
// built-in default rather than 0 (which would cancel every turn on the first poll).
func TestTurnIdleWatchdogDefault(t *testing.T) {
	tun := NewTunables()
	if got, want := tun.TurnIdleWatchdog(), DefaultTurnIdleWatchdogMinutes*time.Minute; got != want {
		t.Fatalf("default idle window = %v, want %v", got, want)
	}
}

// TestTurnIdleWatchdogCappedByHardCeiling verifies an idle window configured above
// the wall-clock cap is pulled down to it — above the cap it could never fire, so
// the wedge detection it configures would silently not exist.
func TestTurnIdleWatchdogCappedByHardCeiling(t *testing.T) {
	tun := NewTunables()
	tun.SetTurnWatchdogMinutes(30)
	tun.SetTurnIdleWatchdogMinutes(90)
	if got, want := tun.TurnIdleWatchdog(), 30*time.Minute; got != want {
		t.Fatalf("idle window above ceiling = %v, want %v", got, want)
	}
}

// TestTurnWatchdogExplicitAboveCeilings verifies the floor only ever raises: a
// watchdog configured above both deadlines is used verbatim.
func TestTurnWatchdogExplicitAboveCeilings(t *testing.T) {
	tun := NewTunables()
	tun.SetTurnWatchdogMinutes(600)
	tun.SetSpawnTimeoutMinutes(120)
	tun.SetScheduleTimeoutMinutes(30)
	if got, want := tun.TurnWatchdog(), 600*time.Minute; got != want {
		t.Fatalf("explicit watchdog = %v, want %v", got, want)
	}
}
