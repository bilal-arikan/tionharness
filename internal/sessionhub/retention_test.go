package sessionhub

import "testing"

func TestCommitReleasesExcessEvents(t *testing.T) {
	h := New("test", 4)
	for range 100 {
		h.Publish("W", "S", KindStep, raw("payload"), false)
	}
	old := h.states[scopeKey("W", "S")].ring
	h.Commit("W", "S")
	state := h.states[scopeKey("W", "S")]
	if len(state.ring) != 4 || cap(state.ring) > 8 {
		t.Fatalf("retained ring len=%d cap=%d", len(state.ring), cap(state.ring))
	}
	for i := 0; i < len(old)-4; i++ {
		if old[i].Payload != nil {
			t.Fatalf("evicted payload %d still retained", i)
		}
	}
	if _, ok := h.Replay("W", "S", 1); ok {
		t.Fatal("an evicted cursor must reset")
	}
	if events, ok := h.Replay("W", "S", 97); !ok || len(events) != 3 {
		t.Fatalf("retained cursor: %d events, ok=%v", len(events), ok)
	}
	if events, ok := h.Replay("W", "S", 0); !ok || len(events) != 0 || cap(events) != 0 {
		t.Fatalf("fresh committed replay allocates history: len=%d cap=%d ok=%v", len(events), cap(events), ok)
	}
}
