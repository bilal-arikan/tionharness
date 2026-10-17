package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/events"
)

// A turn that ends while its workers run on is not the session finishing: the
// chat event carries liveWorkers so the client holds the "done" chime until the
// turn that ends with nothing outstanding.
func TestAnnotateChatCompletionMarksOutstandingWorkers(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	slot := rt.coordSlotFor("SES1")
	slot.workers.Add(2)

	target := map[string]string{"view": "chat", "sessionId": "SES1"}
	got := rt.annotateChatCompletion(events.Event{Type: events.TypeChat, Target: target})
	if got.Target["liveWorkers"] != "2" {
		t.Fatalf("liveWorkers = %q, want 2", got.Target["liveWorkers"])
	}
	if _, mutated := target["liveWorkers"]; mutated {
		t.Fatal("the caller's target map was mutated")
	}

	// An owed drain turn counts too: the session will speak again.
	slot.workers.Add(-2)
	slot.mu.Lock()
	slot.workerPending = true
	slot.mu.Unlock()
	got = rt.annotateChatCompletion(events.Event{Type: events.TypeChat, Target: map[string]string{"sessionId": "SES1"}})
	if got.Target["liveWorkers"] != "1" {
		t.Fatalf("liveWorkers with an owed drain = %q, want 1", got.Target["liveWorkers"])
	}

	// Wake lifecycle phases are not completions.
	got = rt.annotateChatCompletion(events.Event{Type: events.TypeChat, Target: map[string]string{"sessionId": "SES1", "phase": "armed"}})
	if _, ok := got.Target["liveWorkers"]; ok {
		t.Fatal("a wake phase event was annotated")
	}
}

func TestAnnotateChatCompletionLeavesFinishedSessionAlone(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.coordSlotFor("SES1") // coordinated once, nothing outstanding now
	for _, sid := range []string{"SES1", "SES2"} {
		got := rt.annotateChatCompletion(events.Event{Type: events.TypeChat, Target: map[string]string{"sessionId": sid}})
		if _, ok := got.Target["liveWorkers"]; ok {
			t.Fatalf("%s: finished session was annotated", sid)
		}
	}
}
