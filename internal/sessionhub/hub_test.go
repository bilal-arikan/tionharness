package sessionhub

import (
	"encoding/json"
	"testing"
)

func raw(s string) json.RawMessage { return json.RawMessage(`"` + s + `"`) }

// Durable events get a monotonic per-session seq; ephemeral events keep seq 0.
func TestPublishSeq(t *testing.T) {
	h := New("epoch-1", 8)
	if got := h.Publish("S1", KindStep, raw("a"), false); got != 1 {
		t.Fatalf("first durable seq = %d, want 1", got)
	}
	if got := h.Publish("S1", KindStep, raw("b"), false); got != 2 {
		t.Fatalf("second durable seq = %d, want 2", got)
	}
	if got := h.Publish("S1", KindDelta, raw("d"), true); got != 0 {
		t.Fatalf("ephemeral seq = %d, want 0", got)
	}
	// A different session has its own counter.
	if got := h.Publish("S2", KindStep, raw("x"), false); got != 1 {
		t.Fatalf("other-session first seq = %d, want 1", got)
	}
	if h.Head("S1") != 2 {
		t.Fatalf("S1 head = %d, want 2", h.Head("S1"))
	}
}

// Replay returns only events after the cursor; a cursor at head returns empty ok.
func TestReplayFromCursor(t *testing.T) {
	h := New("e", 16)
	for i := 0; i < 5; i++ {
		h.Publish("S", KindStep, raw("s"), false)
	}
	evs, ok := h.Replay("S", 2)
	if !ok {
		t.Fatalf("replay ok=false, want true")
	}
	if len(evs) != 3 || evs[0].Seq != 3 || evs[2].Seq != 5 {
		t.Fatalf("replay from 2 = %+v, want seq 3..5", evs)
	}
	if evs, ok := h.Replay("S", 5); !ok || len(evs) != 0 {
		t.Fatalf("replay from head = %v ok=%v, want empty ok", evs, ok)
	}
}

// A cursor that fell out of the bounded ring yields ok=false → caller must reset.
// Only COMMITTED events are evictable (uncommitted in-flight events are never
// dropped), so eviction is driven here by committing first.
func TestReplayRingEvictionResets(t *testing.T) {
	h := New("e", 4) // keep ~4 durable events
	for i := 0; i < 4; i++ {
		h.Publish("S", KindStep, raw("s"), false) // seq 1..4
	}
	h.Commit("S") // 1..4 committed → now evictable
	for i := 0; i < 4; i++ {
		h.Publish("S", KindStep, raw("t"), false) // seq 5..8, each trims a committed leader
	}
	// Ring now holds ~seq 5..8; a reconnect cursor at seq 2 (evicted) is unrecoverable.
	if _, ok := h.Replay("S", 2); ok {
		t.Fatalf("replay from evicted committed cursor ok=true, want false (reset)")
	}
	// A cursor within the retained window still gap-fills.
	evs, ok := h.Replay("S", 6)
	if !ok || len(evs) != 2 || evs[0].Seq != 7 {
		t.Fatalf("replay from 6 = %+v ok=%v, want seq 7..8", evs, ok)
	}
}

// Uncommitted (in-flight) events are NEVER evicted, even past the ring cap — a
// fresh subscriber must be able to replay the whole running turn.
func TestRingKeepsUncommitted(t *testing.T) {
	h := New("e", 4)
	for i := 0; i < 10; i++ {
		h.Publish("S", KindStep, raw("s"), false) // 10 uncommitted, cap 4
	}
	fresh, ok := h.Replay("S", 0)
	if !ok || len(fresh) != 10 || fresh[0].Seq != 1 || fresh[9].Seq != 10 {
		t.Fatalf("fresh replay dropped in-flight events: %d (want 10)", len(fresh))
	}
}

// A fresh subscribe (since<=0) replays only the in-flight tail (events after the
// last Commit); a reconnect (since>0) still gap-fills from the cursor.
func TestReplayCommitBoundary(t *testing.T) {
	h := New("e", 32)
	for i := 0; i < 3; i++ {
		h.Publish("S", KindStep, raw("s"), false)
	}
	h.Commit("S")                             // seq 1..3 are now in the persisted transcript
	h.Publish("S", KindStep, raw("t"), false) // seq 4 (in-flight)
	h.Publish("S", KindStep, raw("t"), false) // seq 5 (in-flight)

	// Fresh subscribe: only the uncommitted tail (4,5) — a listMessages load
	// already has 1..3, so replaying them would double-render completed turns.
	fresh, ok := h.Replay("S", 0)
	if !ok || len(fresh) != 2 || fresh[0].Seq != 4 || fresh[1].Seq != 5 {
		t.Fatalf("fresh replay = %+v ok=%v, want seq 4..5", fresh, ok)
	}
	// Reconnect from an explicit cursor still gap-fills from there.
	re, ok := h.Replay("S", 3)
	if !ok || len(re) != 2 || re[0].Seq != 4 {
		t.Fatalf("reconnect replay from 3 = %+v ok=%v, want seq 4..5", re, ok)
	}
	// After committing everything, a fresh subscribe replays nothing.
	h.Commit("S")
	if evs, ok := h.Replay("S", 0); !ok || len(evs) != 0 {
		t.Fatalf("fresh replay after full commit = %v ok=%v, want empty", evs, ok)
	}
}

// Subscribers receive live durable + ephemeral events; unsubscribe closes the channel.
func TestSubscribeReceivesLive(t *testing.T) {
	h := New("e", 8)
	id, ch, head := h.Subscribe("S")
	if head != 0 {
		t.Fatalf("fresh head = %d, want 0", head)
	}
	h.Publish("S", KindStep, raw("live"), false)
	select {
	case ev := <-ch:
		if ev.Seq != 1 || ev.Kind != KindStep {
			t.Fatalf("received %+v, want seq 1 step", ev)
		}
	default:
		t.Fatalf("no live event delivered to subscriber")
	}
	if n := h.SubscriberCount("S"); n != 1 {
		t.Fatalf("subscriber count = %d, want 1", n)
	}
	h.Unsubscribe("S", id)
	if _, open := <-ch; open {
		t.Fatalf("channel still open after unsubscribe")
	}
	if n := h.SubscriberCount("S"); n != 0 {
		t.Fatalf("subscriber count after unsub = %d, want 0", n)
	}
}

// Drop releases a deleted session's state: the ring, the seq counter and the
// subscriber channels. Without it the states map grows for the process lifetime.
func TestDropReleasesSessionState(t *testing.T) {
	h := New("e", 8)
	id, ch, _ := h.Subscribe("S")
	h.Publish("S", KindStep, raw("a"), false)
	h.Publish("S", KindStep, raw("b"), false)
	if len(h.states) != 1 {
		t.Fatalf("states = %d, want 1", len(h.states))
	}

	h.Drop("S")

	if len(h.states) != 0 {
		t.Fatalf("states after drop = %d, want 0", len(h.states))
	}
	// Watching windows must be released, not left hanging on a dead session.
	for {
		if _, open := <-ch; !open {
			break
		}
	}
	// A late Unsubscribe for the dropped session must not panic (double close).
	h.Unsubscribe("S", id)
	// And the counters are genuinely gone, not just hidden.
	if got := h.Head("S"); got != 0 {
		t.Fatalf("head after drop = %d, want 0", got)
	}
	if got := h.SubscriberCount("S"); got != 0 {
		t.Fatalf("subscriber count after drop = %d, want 0", got)
	}
}

// Drop is safe on a nil hub and on a session that was never seen.
func TestDropIsSafeWhenAbsent(t *testing.T) {
	var nilHub *Hub
	nilHub.Drop("S") // must not panic
	h := New("e", 8)
	h.Drop("")
	h.Drop("never-published")
	if len(h.states) != 0 {
		t.Fatalf("dropping an absent session created state: %d", len(h.states))
	}
}
