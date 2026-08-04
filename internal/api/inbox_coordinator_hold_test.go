package api

import (
	"sync/atomic"
	"testing"
	"time"
)

// waitInboxDrained polls until a session's queue is empty AND its worker has stopped,
// or fails after a short timeout. The self-kick path dispatches on a fresh goroutine,
// so a synchronous state read right after runInboxWorker returns would race it.
func waitInboxDrained(t *testing.T, s *Server, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.inbox.lock()
		ib := s.inbox.sessions[sessionID]
		drained := ib != nil && len(ib.items) == 0 && !ib.running
		s.inbox.unlock()
		if drained {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("inbox did not drain within the timeout")
}

// TestInboxHoldsWhileCoordinatorWorkersBusy: when the worker-liveness probe reports
// busy, the serial worker must PARK — leave the queued item in place (visible in the
// tray) and mark itself not-running — instead of dispatching a coordinator turn mid
// worker-run. This is the core of the "Sıraya while workers run" behavior.
func TestInboxHoldsWhileCoordinatorWorkersBusy(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	s.workersBusyFn = func(_, _ string) bool { return true } // workers always running

	s.inbox.sessions["SES"] = &sessionInbox{
		seen:    map[string]bool{"a": true},
		items:   []inboxItem{{ClientMsgID: "a"}},
		running: true, // kickInbox already claimed it (as the enqueue path would)
	}

	// Runs on the caller goroutine so it returns once parked (no dispatch is reached).
	s.runInboxWorker("SES")

	s.inbox.lock()
	ib := s.inbox.sessions["SES"]
	running := ib.running
	itemCount := len(ib.items)
	inflight := ib.inflight
	s.inbox.unlock()

	if running {
		t.Fatal("worker must mark itself not-running when it parks on busy workers")
	}
	if itemCount != 1 {
		t.Fatalf("parked message must stay queued (visible in the tray); got %d items", itemCount)
	}
	if inflight != nil {
		t.Fatal("a parked message must NOT be moved into the in-flight slot")
	}
}

// TestInboxSelfKicksWhenWorkersDrainDuringPark: if the workers finish in the window
// between the busy probe and the worker marking itself not-running, the completion
// kick may have no-op'd (the worker was still flagged running). The park path must
// re-probe and self-kick so the message is never stranded. Here the probe reports
// busy on the FIRST call (drives the park) and drained on every call after, so the
// re-check inside the park path must restart the worker.
func TestInboxSelfKicksWhenWorkersDrainDuringPark(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	var calls int32
	// First probe (dispatch decision) → busy; the re-check after parking → drained.
	s.workersBusyFn = func(_, _ string) bool { return atomic.AddInt32(&calls, 1) == 1 }

	s.inbox.sessions["SES"] = &sessionInbox{
		seen:    map[string]bool{"a": true},
		items:   []inboxItem{{ClientMsgID: "a"}}, // no WorkspaceID → dispatch clears inflight, loops, drains
		running: true,
	}

	// Parks on the first probe, then the re-check sees drained and self-kicks a fresh
	// worker goroutine — so wait for that goroutine to finish draining.
	s.runInboxWorker("SES")
	waitInboxDrained(t, s, "SES")

	if atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("park path must re-probe worker liveness; probes=%d", atomic.LoadInt32(&calls))
	}
}

// TestInboxDispatchesWhenNoCoordinatorWorkers: the fast path is unchanged — a plain
// session (probe reports not busy) drains its queue immediately, never parking.
func TestInboxDispatchesWhenNoCoordinatorWorkers(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	s.workersBusyFn = func(_, _ string) bool { return false }

	s.inbox.sessions["SES"] = &sessionInbox{
		seen:    map[string]bool{"a": true},
		items:   []inboxItem{{ClientMsgID: "a"}}, // no WorkspaceID → dispatch clears inflight and loops to drain
		running: true,
	}

	s.runInboxWorker("SES")

	s.inbox.lock()
	ib := s.inbox.sessions["SES"]
	itemCount := len(ib.items)
	running := ib.running
	s.inbox.unlock()

	if running {
		t.Fatal("worker must stop after draining")
	}
	if itemCount != 0 {
		t.Fatalf("a non-coordinator queue must dispatch immediately; %d items left", itemCount)
	}
}
