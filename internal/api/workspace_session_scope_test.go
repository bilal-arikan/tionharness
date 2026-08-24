package api

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/sessionhub"
)

// Session ids are allocated per workspace store, so "SES1" is the first session of
// EVERY workspace. These tests lock the scoping that keeps two same-numbered
// sessions apart in the SERVER-WIDE structures — the bug where opening a freshly
// created workspace replayed another workspace's in-flight turn, and a message
// sent in one could be queued (and persisted) into the other.

// TestHubScopedByWorkspace: a window watching WS2/SES1 must receive nothing from
// WS1/SES1 — neither live frames nor a fresh subscriber's in-flight replay.
func TestHubScopedByWorkspace(t *testing.T) {
	h := sessionhub.New("t", 0)
	_, ch2, head := h.Subscribe("WS2", "SES1")
	if head != 0 {
		t.Fatalf("a fresh workspace's session starts at head 0, got %d", head)
	}

	h.Publish("WS1", "SES1", sessionhub.KindStep, json.RawMessage(`{"kind":"text"}`), false)

	select {
	case ev := <-ch2:
		t.Fatalf("WS2 subscriber received WS1's event: %+v", ev)
	default:
	}
	// The in-flight tail a fresh subscriber replays is per (workspace, session) too.
	if evs, ok := h.Replay("WS2", "SES1", 0); !ok || len(evs) != 0 {
		t.Fatalf("WS2/SES1 replay = %v (ok=%v), want empty", evs, ok)
	}
	if evs, ok := h.Replay("WS1", "SES1", 0); !ok || len(evs) != 1 {
		t.Fatalf("WS1/SES1 must still replay its own in-flight event, got %v", evs)
	}
}

// TestInboxScopedByWorkspace: enqueuing into WS1/SES1 must not create or touch the
// queue of WS2's same-numbered session.
func TestInboxScopedByWorkspace(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	// No workspaces manager: the turn cannot dispatch (workspaceByID returns nil),
	// which is fine — this asserts the QUEUE routing, not the run.
	s.enqueueMessage("WS1", chatReq{SessionID: "SES1", Message: "merhaba"}, "m1")

	s.inbox.lock()
	defer s.inbox.unlock()
	if ib := s.inbox.at("WS1", "SES1"); ib == nil || ib.wsID != "WS1" {
		t.Fatal("WS1/SES1 must own the enqueued message")
	}
	if ib := s.inbox.at("WS2", "SES1"); ib != nil {
		t.Fatal("WS2/SES1 must have no queue of its own")
	}
}

// TestPermGrantsScopedByWorkspace: an "always allow" granted in one workspace's
// session must not silently apply to a same-numbered session elsewhere.
func TestPermGrantsScopedByWorkspace(t *testing.T) {
	p := newPermGrantStore()
	if p.forSession("WS1", "SES1") == p.forSession("WS2", "SES1") {
		t.Fatal("same-numbered sessions in different workspaces must not share grants")
	}
	if p.forSession("WS1", "SES1") != p.forSession("WS1", "SES1") {
		t.Fatal("the same session must keep one grant set across turns")
	}
}

// TestInteractionsScopedByWorkspace: an open ask/permission card is addressable
// only within its own workspace, so another workspace's window cannot resolve it.
func TestInteractionsScopedByWorkspace(t *testing.T) {
	srv := &Server{hub: sessionhub.New("t", 0), interactions: newInteractionStore()}
	pi := srv.openInteraction("WS1", "SES1", "ask", map[string]any{"question": "q"})

	if srv.resolveInteraction("WS2", "SES1", pi.id, "hijacked", "w") {
		t.Fatal("another workspace must not resolve this card")
	}
	if !srv.resolveInteraction("WS1", "SES1", pi.id, "ok", "w") {
		t.Fatal("the owning workspace must resolve its own card")
	}
}

// TestChatRunsScopedByWorkspace: an in-flight turn is reported only for its own
// workspace — otherwise a delete elsewhere blocks on it, and a stop cancels it.
func TestChatRunsScopedByWorkspace(t *testing.T) {
	runs := newChatRuns()
	runs.register("R1", "SES1", "WS1", func() {})

	if _, live := runs.sessionRunInfo("WS2", "SES1"); live {
		t.Fatal("WS2/SES1 must not see WS1's running turn")
	}
	if _, live := runs.sessionRunInfo("WS1", "SES1"); !live {
		t.Fatal("WS1/SES1 must see its own running turn")
	}
	// The Interaction MCP Bearer secret is per (workspace, session, agent) too.
	if runs.interactionToken("WS1", "SES1", "AGT1") == runs.interactionToken("WS2", "SES1", "AGT1") {
		t.Fatal("same-numbered sessions in different workspaces must not share a token")
	}
}
