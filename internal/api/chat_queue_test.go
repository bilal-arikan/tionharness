package api

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/sessionhub"
)

// TestPayloadClientMsgID extracts the correlation id a terminal hub event carries,
// returning "" when absent or the payload is empty.
func TestPayloadClientMsgID(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"present", `{"sessionTitle":"t","clientMsgId":"abc"}`, "abc"},
		{"absent", `{"sessionTitle":"t"}`, ""},
		{"empty", ``, ""},
		{"error-shape", `{"error":"boom","reason":"panic","clientMsgId":"xyz"}`, "xyz"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := payloadClientMsgID(json.RawMessage(c.raw)); got != c.want {
				t.Fatalf("payloadClientMsgID(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

// TestRelayLegacyFrameOwnTerminalOnly is the correlation guard for the durable
// /chat/stream cutover: the relay must map activity frames to the legacy SSE shape
// and close ONLY on ITS OWN turn's terminal event — a turn_done/turn_error from a
// turn queued ahead (a different clientMsgId) must NOT emit a legacy "done"/"error"
// (which would close an old client early) nor signal completion.
func TestRelayLegacyFrameOwnTerminalOnly(t *testing.T) {
	type frame struct {
		event string
		data  any
	}
	mine := "mine-123"

	newRelay := func() (*[]frame, func(string, any)) {
		var out []frame
		return &out, func(event string, data any) { out = append(out, frame{event, data}) }
	}
	ev := func(kind, payload string) sessionhub.Event {
		return sessionhub.Event{Kind: kind, Payload: json.RawMessage(payload)}
	}

	// Activity frames map to legacy events and never signal completion.
	for _, c := range []struct{ kind, event string }{
		{sessionhub.KindUserMessage, "meta"},
		{sessionhub.KindAgentStart, "agent"},
		{sessionhub.KindStep, "step"},
		{sessionhub.KindReply, "reply"},
	} {
		out, write := newRelay()
		if done := relayLegacyFrame(write, ev(c.kind, `{}`), mine); done {
			t.Fatalf("%s must not be terminal", c.kind)
		}
		if len(*out) != 1 || (*out)[0].event != c.event {
			t.Fatalf("%s → expected one %q frame, got %+v", c.kind, c.event, *out)
		}
	}

	// A terminal event from ANOTHER queued turn must be dropped, not close the stream.
	out, write := newRelay()
	if done := relayLegacyFrame(write, ev(sessionhub.KindTurnDone, `{"clientMsgId":"other"}`), mine); done {
		t.Fatal("turn_done from another turn must not close the stream")
	}
	if len(*out) != 0 {
		t.Fatalf("another turn's turn_done must emit no legacy frame, got %+v", *out)
	}

	// Our own terminal events close the stream and emit the matching legacy frame.
	out, write = newRelay()
	if done := relayLegacyFrame(write, ev(sessionhub.KindTurnDone, `{"clientMsgId":"mine-123"}`), mine); !done {
		t.Fatal("our turn_done must close the stream")
	}
	if len(*out) != 1 || (*out)[0].event != "done" {
		t.Fatalf("our turn_done → expected one \"done\" frame, got %+v", *out)
	}

	out, write = newRelay()
	if done := relayLegacyFrame(write, ev(sessionhub.KindTurnError, `{"clientMsgId":"mine-123","error":"boom"}`), mine); !done {
		t.Fatal("our turn_error must close the stream")
	}
	if len(*out) != 1 || (*out)[0].event != "error" {
		t.Fatalf("our turn_error → expected one \"error\" frame, got %+v", *out)
	}
}

// TestQueueHasMsg guards the cancel-detection signal: a message counts as live while
// it is WAITING in the queue or is the dispatched inflight head; absent from both it
// is gone (cancelled/cleared). A malformed payload is treated as present so a bad
// frame never triggers a false "cancelled".
func TestQueueHasMsg(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		id   string
		want bool
	}{
		{"waiting", `{"queue":[{"clientMsgId":"a"},{"clientMsgId":"me"}],"inflightClientMsgId":""}`, "me", true},
		{"inflight", `{"queue":[],"inflightClientMsgId":"me"}`, "me", true},
		{"absent", `{"queue":[{"clientMsgId":"a"}],"inflightClientMsgId":"b"}`, "me", false},
		{"empty-queue", `{"queue":[],"inflightClientMsgId":""}`, "me", false},
		{"malformed", `not json`, "me", true},
		{"nil", ``, "me", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := queueHasMsg([]byte(c.raw), c.id); got != c.want {
				t.Fatalf("queueHasMsg(%q,%q) = %v, want %v", c.raw, c.id, got, c.want)
			}
		})
	}
}

// TestDrainReplayRecoversDroppedTerminal is the safety-net guard: when the hub's
// non-blocking fan-out drops a turn's terminal frame (no later event follows to
// trigger gap detection), the ping-tick drainReplay must still pull it from the ring
// and signal completion — otherwise a queue observer (/chat + /chat/stream) hangs.
func TestDrainReplayRecoversDroppedTerminal(t *testing.T) {
	s := &Server{hub: sessionhub.New("epoch", 512)}
	sid := "SES1"
	// A turn's durable events land in the ring; simulate the observer having seen
	// NONE of them (lastSeq stays 0, as if every live frame was dropped).
	s.hub.Publish("WS1", sid, sessionhub.KindStep, json.RawMessage(`{}`), false)
	s.hub.Publish("WS1", sid, sessionhub.KindStep, json.RawMessage(`{}`), false)
	s.hub.Publish("WS1", sid, sessionhub.KindTurnDone, json.RawMessage(`{"clientMsgId":"me"}`), false)

	var seen []string
	lastSeq := int64(0)
	done := s.drainReplay("WS1", sid, &lastSeq, func(ev sessionhub.Event) bool {
		seen = append(seen, ev.Kind)
		return ev.Kind == sessionhub.KindTurnDone && payloadClientMsgID(ev.Payload) == "me"
	})
	if !done {
		t.Fatal("drainReplay must recover the dropped terminal and signal done")
	}
	if len(seen) != 3 || seen[2] != sessionhub.KindTurnDone {
		t.Fatalf("expected step,step,turn_done, got %v", seen)
	}
	// *lastSeq advances past the two steps but not the terminal (it returns first).
	if lastSeq != 2 {
		t.Fatalf("lastSeq should advance to 2 (last non-terminal), got %d", lastSeq)
	}
}
