package sessionhub

import (
	"testing"
	"time"
)

// TestLastActivityTracksEveryPublish verifies the liveness stamp advances on
// EPHEMERAL events too. The queue's idle watchdog reads this signal, and a turn
// streaming tokens without emitting a durable step is very much alive — stamping
// only durable events would let the watchdog kill it as if it were wedged.
func TestLastActivityTracksEveryPublish(t *testing.T) {
	h := New("epoch", 8)
	if _, ok := h.LastActivity("WS1", "S"); ok {
		t.Fatal("a session with no events must report no activity")
	}

	h.Publish("WS1", "S", KindStep, raw("s"), false)
	first, ok := h.LastActivity("WS1", "S")
	if !ok {
		t.Fatal("durable event did not stamp activity")
	}

	time.Sleep(2 * time.Millisecond)
	h.Publish("WS1", "S", KindDelta, raw("tok"), true) // ephemeral: seq stays put
	second, ok := h.LastActivity("WS1", "S")
	if !ok || !second.After(first) {
		t.Fatalf("ephemeral event did not advance activity (first=%v second=%v)", first, second)
	}
	if h.Head("WS1", "S") != 1 {
		t.Fatalf("ephemeral event must not bump seq, head = %d", h.Head("WS1", "S"))
	}
}

// TestLastActivityScopedByWorkspace verifies the stamp is per (workspace, session):
// session ids repeat across stores, so a workspace-blind lookup would report one
// workspace's traffic as another's liveness and keep a wedged turn alive forever.
func TestLastActivityScopedByWorkspace(t *testing.T) {
	h := New("epoch", 8)
	h.Publish("WS1", "S", KindStep, raw("s"), false)
	if _, ok := h.LastActivity("WS2", "S"); ok {
		t.Fatal("WS2 must not inherit WS1's activity for the same session id")
	}
}
