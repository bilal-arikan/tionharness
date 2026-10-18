package agent

import (
	"testing"
	"time"
)

func TestTurnIdleTimeoutDefault(t *testing.T) {
	tun := NewTunables()
	if got, want := tun.TurnIdleTimeout(), DefaultTurnIdleTimeoutMinutes*time.Minute; got != want {
		t.Fatalf("default idle = %v, want %v", got, want)
	}
	if got, want := tun.QueueIdleBackstop(), DefaultQueueIdleBackstop; got != want {
		t.Fatalf("default backstop = %v, want %v", got, want)
	}
}

// TestQueueIdleBackstopTrailsTurnIdle pins the ordering the merge relies on: the
// queue's outer watchdog is only a backstop, so it must outlast the turn's own cut
// whatever the configured window is.
func TestQueueIdleBackstopTrailsTurnIdle(t *testing.T) {
	tun := NewTunables()
	tun.SetTurnIdleTimeoutMinutes(90)
	if got := tun.TurnIdleTimeout(); got != 90*time.Minute {
		t.Fatalf("idle = %v, want 90m", got)
	}
	if got := tun.QueueIdleBackstop(); got <= tun.TurnIdleTimeout() {
		t.Fatalf("backstop %v does not trail the turn idle window %v", got, tun.TurnIdleTimeout())
	}
}
