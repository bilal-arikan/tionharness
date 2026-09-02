package api

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
)

// TestSSEEventNameSkipsWorkspaceStream: workspace-stream types never reach the
// global notification feed (they would toast as unknown notifications), while
// the existing routing of step/flow/log frames is unchanged.
func TestSSEEventNameSkipsWorkspaceStream(t *testing.T) {
	cases := map[string]struct {
		name string
		skip bool
	}{
		events.TypeChat:               {"notify", false},
		events.TypeSessionStep:        {"step", false},
		events.TypeFlowNode:           {"flownode", false},
		"flow_node_step":              {"flownodestep", false},
		events.TypeLog:                {"log", false},
		events.TypeWSSessionLifecycle: {"", true},
		events.TypeWSFlowRun:          {"", true},
		events.TypeWSTrajectory:       {"", true},
		events.TypeWSBoard:            {"", true},
	}
	for typ, want := range cases {
		name, skip := sseEventName(typ)
		if name != want.name || skip != want.skip {
			t.Fatalf("sseEventName(%q) = (%q, %v), want (%q, %v)", typ, name, skip, want.name, want.skip)
		}
	}
}

// TestBridgeWorkspaceEvent: a ws:* bus event lands on the hub's workspace scope
// with its kind stripped of the prefix and its target + data preserved; an
// unstamped event is dropped rather than guessed into a workspace.
func TestBridgeWorkspaceEvent(t *testing.T) {
	s := newTestServer()
	s.hub = sessionhub.New("epoch", 8)

	s.bridgeWorkspaceEvent(events.Event{
		Type: events.TypeWSFlowRun, Level: "info", WorkspaceID: "WS1",
		Target: map[string]string{"flowRunId": "RUN1", "status": "running"},
		Data:   json.RawMessage(`{"runId":"RUN1","status":"running"}`),
	})
	s.bridgeWorkspaceEvent(events.Event{Type: events.TypeWSFlowRun, Target: map[string]string{"flowRunId": "RUN2"}}) // no workspace → dropped

	if head := s.hub.HeadWorkspace("WS1"); head != 1 {
		t.Fatalf("workspace head = %d, want 1 (the unstamped event must be dropped)", head)
	}
	evs, ok := s.hub.ReplayWorkspace("WS1", 0)
	if !ok || len(evs) != 0 {
		t.Fatalf("fresh replay must be empty, got %d ok=%v", len(evs), ok)
	}
	s.bridgeWorkspaceEvent(events.Event{Type: events.TypeWSSessionLifecycle, WorkspaceID: "WS1", Target: map[string]string{"sessionId": "SES1", "op": "create"}, Data: json.RawMessage(`{"sessionId":"SES1"}`)})
	evs, ok = s.hub.ReplayWorkspace("WS1", 1)
	if !ok || len(evs) != 1 {
		t.Fatalf("reconnect replay = %d ok=%v, want 1", len(evs), ok)
	}
	ev := evs[0]
	if ev.Kind != sessionhub.KindWSSessionLifecycle || ev.SessionID != "" || ev.Seq != 2 {
		t.Fatalf("bridged event = %+v, want kind session_lifecycle seq 2 no session id", ev)
	}
	var payload workspaceStreamPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("payload decode: %v", err)
	}
	if payload.Target["sessionId"] != "SES1" || payload.Target["op"] != "create" || string(payload.Data) != `{"sessionId":"SES1"}` {
		t.Fatalf("payload = %+v, want target + data preserved", payload)
	}
}
