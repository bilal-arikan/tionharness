package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/sessionhub"
)

// TestTurnIdleForFallsBackToStart verifies a turn that has published nothing yet is
// still measurable: idleness counts from the turn's start, so a turn that wedges
// during setup (before it can emit a single step) is reclaimed rather than being
// treated as "no signal, leave it alone".
func TestTurnIdleForFallsBackToStart(t *testing.T) {
	s := &Server{hub: sessionhub.New("e", 8)}
	started := time.Now().Add(-90 * time.Second)

	idle, ok := s.turnIdleFor("WS1", "SES", started)
	if !ok {
		t.Fatal("idle must be measurable from the start time alone")
	}
	if idle < 90*time.Second {
		t.Fatalf("idle = %v, want >= 90s measured from start", idle)
	}
}

// TestTurnIdleForResetsOnActivity is the core of the idle watchdog: a turn that is
// emitting is NOT idle, no matter how long it has been running. This is what lets
// the wall-clock ceiling be generous without a long, productive turn (a build/test
// loop streaming tool calls) being cut as if it were hung.
func TestTurnIdleForResetsOnActivity(t *testing.T) {
	hub := sessionhub.New("e", 8)
	s := &Server{hub: hub}
	started := time.Now().Add(-30 * time.Minute)

	hub.Publish("WS1", "SES", sessionhub.KindStep, json.RawMessage(`"tool"`), false)

	idle, ok := s.turnIdleFor("WS1", "SES", started)
	if !ok {
		t.Fatal("idle must be measurable after an event")
	}
	if idle > time.Second {
		t.Fatalf("idle = %v, want ~0 — the turn just emitted a step", idle)
	}
}

// TestTurnIdleForIgnoresOtherWorkspace: session ids repeat across stores, so a busy
// WS2/SES must not keep a silent WS1/SES alive — that would defeat the watchdog for
// exactly the sessions most likely to collide.
func TestTurnIdleForIgnoresOtherWorkspace(t *testing.T) {
	hub := sessionhub.New("e", 8)
	s := &Server{hub: hub}
	started := time.Now().Add(-10 * time.Minute)

	hub.Publish("WS2", "SES", sessionhub.KindStep, json.RawMessage(`"tool"`), false)

	idle, _ := s.turnIdleFor("WS1", "SES", started)
	if idle < 10*time.Minute {
		t.Fatalf("idle = %v, want >= 10m — WS2's traffic is not WS1's liveness", idle)
	}
}

// TestInboxWatchdogDefaultsWithoutTunables verifies the watchdog degrades to the
// built-in bounds rather than to a zero duration when no Tunables is wired (the
// shape used by several tests): a zero ceiling would cancel every turn instantly.
func TestInboxWatchdogDefaultsWithoutTunables(t *testing.T) {
	s := &Server{}
	if got, want := s.inboxTurnWatchdog(), agent.DefaultTurnWatchdogMinutes*time.Minute; got != want {
		t.Fatalf("hard ceiling = %v, want %v", got, want)
	}
	if got, want := s.inboxTurnIdleWatchdog(), agent.DefaultTurnIdleWatchdogMinutes*time.Minute; got != want {
		t.Fatalf("idle window = %v, want %v", got, want)
	}
}
