package sessionhub

import "testing"

// TestWorkspacePublishAndReplay: the workspace scope has its own seq, replays
// nothing to a fresh subscriber, gap-fills a reconnect, and never leaks into a
// session scope or another workspace.
func TestWorkspacePublishAndReplay(t *testing.T) {
	h := New("epoch-1", 4)
	if got := h.PublishWorkspace("WS1", KindWSFlowRun, raw("a")); got != 1 {
		t.Fatalf("first workspace seq = %d, want 1", got)
	}
	if got := h.PublishWorkspace("WS1", KindWSSessionLifecycle, raw("b")); got != 2 {
		t.Fatalf("second workspace seq = %d, want 2", got)
	}
	if h.HeadWorkspace("WS1") != 2 {
		t.Fatalf("head = %d, want 2", h.HeadWorkspace("WS1"))
	}
	// Fresh subscribe replays nothing: every workspace publish is committed.
	if evs, ok := h.ReplayWorkspace("WS1", 0); !ok || len(evs) != 0 {
		t.Fatalf("fresh replay = %d events ok=%v, want 0 ok", len(evs), ok)
	}
	// Reconnect from a cursor gap-fills.
	evs, ok := h.ReplayWorkspace("WS1", 1)
	if !ok || len(evs) != 1 || evs[0].Seq != 2 || evs[0].Kind != KindWSSessionLifecycle || evs[0].SessionID != "" {
		t.Fatalf("reconnect replay = %+v ok=%v, want the seq-2 event with an empty SessionID", evs, ok)
	}
	// Isolation: no bleed into a session scope of the same workspace nor another workspace.
	if h.Head("WS1", "SES1") != 0 || h.HeadWorkspace("WS2") != 0 {
		t.Fatalf("workspace publishes leaked: session head %d, WS2 head %d", h.Head("WS1", "SES1"), h.HeadWorkspace("WS2"))
	}
	if got := h.PublishWorkspace("", KindWSFlowRun, raw("x")); got != 0 {
		t.Fatalf("empty workspace must publish nothing, got seq %d", got)
	}
}

// TestWorkspaceRingIsLargerAndResetsWhenExceeded: the workspace ring keeps
// workspaceRingFactor × ringCap committed events; a cursor older than that
// reports ok=false (reset) instead of a silent gap.
func TestWorkspaceRingIsLargerAndResetsWhenExceeded(t *testing.T) {
	h := New("e", 4) // session ring 4 → workspace ring 16
	for i := 0; i < 20; i++ {
		h.PublishWorkspace("WS1", KindWSFlowRun, raw("s"))
	}
	if evs, ok := h.ReplayWorkspace("WS1", 4); !ok || len(evs) != 16 {
		t.Fatalf("replay since=4: %d events ok=%v, want 16 (ring holds seq 5..20)", len(evs), ok)
	}
	if _, ok := h.ReplayWorkspace("WS1", 3); ok {
		t.Fatal("a cursor that fell out of the ring must request a reset")
	}
}

// TestWorkspaceSubscribeReceivesLive: a subscriber gets live events with seq,
// and unsubscribe closes its channel.
func TestWorkspaceSubscribeReceivesLive(t *testing.T) {
	h := New("e", 8)
	h.PublishWorkspace("WS1", KindWSFlowRun, raw("before"))
	id, ch, head := h.SubscribeWorkspace("WS1")
	if head != 1 {
		t.Fatalf("head at subscribe = %d, want 1", head)
	}
	h.PublishWorkspace("WS1", KindWSTrajectory, raw("live"))
	ev := <-ch
	if ev.Seq != 2 || ev.Kind != KindWSTrajectory {
		t.Fatalf("live event = %+v, want seq 2 trajectory", ev)
	}
	h.UnsubscribeWorkspace("WS1", id)
	if _, open := <-ch; open {
		t.Fatal("channel must be closed after unsubscribe")
	}
	h.DropWorkspace("WS1")
	if h.HeadWorkspace("WS1") != 0 {
		t.Fatal("drop must discard the workspace scope")
	}
}
