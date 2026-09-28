package api

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/sessionhub"
)

// lastQueueUpdateSteerable reads the "steerable" field of the most recent
// queue_update the session published. It fails the test when no queue_update was
// published at all, and when the field is missing — an absent field is exactly
// the bug this flag exists to prevent (the client would fall back to "cannot
// steer" and disable the action for every turn).
func lastQueueUpdateSteerable(t *testing.T, s *Server, wsID, sessionID string) bool {
	t.Helper()
	events, ok := s.hub.Replay(wsID, sessionID, 0)
	if !ok {
		t.Fatalf("hub replay unavailable for %s/%s", wsID, sessionID)
	}
	var payload map[string]json.RawMessage
	found := false
	for _, ev := range events {
		if ev.Kind != sessionhub.KindQueueUpdate {
			continue
		}
		payload = nil
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatalf("decode queue_update payload: %v", err)
		}
		found = true
	}
	if !found {
		t.Fatal("no queue_update was published for this session")
	}
	raw, ok := payload["steerable"]
	if !ok {
		t.Fatal(`queue_update carries no "steerable" field: the tray cannot tell a ` +
			`steerable turn from one that can only answer "unsupported"`)
	}
	var steerable bool
	if err := json.Unmarshal(raw, &steerable); err != nil {
		t.Fatalf("decode steerable: %v", err)
	}
	return steerable
}

// TestQueueUpdateReportsSteerableTurn: with a run whose provider/mode CAN carry
// mid-turn guidance, the session's queue_update must say so, or the tray disables
// an action that would in fact have worked.
func TestQueueUpdateReportsSteerableTurn(t *testing.T) {
	s, wsp, sessionID, run := steerQueuedFixture(t, "use the other endpoint")
	run.setSteerable(true)

	s.republishQueue(wsp.ID, sessionID)

	if !lastQueueUpdateSteerable(t, s, wsp.ID, sessionID) {
		t.Error("steerable = false for a run that accepts a steer: the tray would " +
			"disable a working action")
	}
}

// TestQueueUpdateReportsUnsteerableTurn is the case the card is about: a turn IS
// running (so the old "is it streaming?" gate said yes) but it has no boundary a
// steer could ride, such as a CLI turn without the bridge. The published
// flag must be false so the tray disables the action instead of offering an
// operation the backend can only refuse as "unsupported".
func TestQueueUpdateReportsUnsteerableTurn(t *testing.T) {
	s, wsp, sessionID, run := steerQueuedFixture(t, "check the other file first")
	run.setProvider("claude-cli")
	run.setSteerable(false)

	s.republishQueue(wsp.ID, sessionID)

	if lastQueueUpdateSteerable(t, s, wsp.ID, sessionID) {
		t.Error(`steerable = true for a run that answers "unsupported": the tray ` +
			"would offer a steer that cannot land")
	}
}

// TestQueueUpdateNotSteerableWithoutRunningTurn: no in-flight turn means there is
// nothing to steer. The flag must still be PRESENT and false rather than omitted,
// so the client never has to guess.
func TestQueueUpdateNotSteerableWithoutRunningTurn(t *testing.T) {
	s, wsp, sessionID, _ := steerQueuedFixture(t, "no turn yet")
	// Drop the registered run: the fixture's session now has a held turn slot but
	// no run that owns an in-flight turn.
	s.runs.unregister("r-q")

	s.republishQueue(wsp.ID, sessionID)

	if lastQueueUpdateSteerable(t, s, wsp.ID, sessionID) {
		t.Error("steerable = true with no in-flight turn")
	}
}
