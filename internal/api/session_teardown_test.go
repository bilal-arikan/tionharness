package api

import (
	"testing"
	"time"
)

// TestTeardown_NoLiveRun_RemovesInbox: with no in-flight turn (and no workspace), a
// delete's teardown drops the in-memory inbox entry so a serial worker can never
// re-dispatch a turn for a session that no longer exists.
func TestTeardown_NoLiveRun_RemovesInbox(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	s.inbox.sessions["SES"] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "a"}}}

	if err := s.teardownSessionRuntime(nil, "SES"); err != nil {
		t.Fatalf("teardown returned error: %v", err)
	}
	s.inbox.lock()
	_, ok := s.inbox.sessions["SES"]
	s.inbox.unlock()
	if ok {
		t.Fatal("inbox entry must be removed after a successful teardown")
	}
}

// TestStopInflightTurn_Stops: a turn whose goroutine unwinds (run.done closes on
// unregister) is reported stopped — the happy path where cancel actually kills it.
func TestStopInflightTurn_Stops(t *testing.T) {
	s := &Server{runs: newChatRuns()}
	s.runs.register("R1", "SES", "WS", func() {})
	go func() {
		time.Sleep(10 * time.Millisecond)
		s.runs.unregister("R1") // simulates the turn goroutine returning after cancel
	}()
	if err := s.stopInflightTurn("SES", 2*time.Second); err != nil {
		t.Fatalf("want stopped, got error: %v", err)
	}
}

// TestStopInflightTurn_Timeout: a turn that ignores cancellation (never unregisters)
// is reported un-stoppable, which is exactly what makes the caller ABORT the delete
// instead of stranding the process.
func TestStopInflightTurn_Timeout(t *testing.T) {
	s := &Server{runs: newChatRuns()}
	s.runs.register("R1", "SES", "WS", func() {})
	if err := s.stopInflightTurn("SES", 50*time.Millisecond); err == nil {
		t.Fatal("want a timeout error for a turn that never stops")
	}
}

// TestKickInbox_FrozenDoesNotDispatch: while a teardown holds the closing freeze, the
// serial worker must not start — otherwise it would spawn a fresh turn (and subprocess)
// for a session mid-delete.
func TestKickInbox_FrozenDoesNotDispatch(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	s.inbox.sessions["SES"] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "a"}}, closing: true}

	s.kickInbox("SES")
	s.inbox.lock()
	running := s.inbox.sessions["SES"].running
	s.inbox.unlock()
	if running {
		t.Fatal("a frozen (closing) inbox must not start a worker")
	}
}

// TestResumeInboxAfterAbortedTeardown_Unfreezes: when a delete aborts, the freeze is
// lifted so the preserved queue can drain again (session lives on).
func TestResumeInboxAfterAbortedTeardown_Unfreezes(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	s.inbox.sessions["SES"] = &sessionInbox{seen: map[string]bool{}, closing: true} // no items → kick is a no-op

	s.resumeInboxAfterAbortedTeardown("SES")
	s.inbox.lock()
	closing := s.inbox.sessions["SES"].closing
	s.inbox.unlock()
	if closing {
		t.Fatal("resume must clear the closing freeze")
	}
}
