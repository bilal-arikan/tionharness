package api

import (
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// testWS is the minimal workspace a teardown needs: an id (everything per-session
// is keyed by it) and no runtime/DB, so the phases that touch those are skipped.
func testWS(id string) *workspace.Workspace {
	return &workspace.Workspace{Meta: workspace.Meta{ID: id}}
}

// TestTeardown_NoLiveRun_RemovesInbox: with no in-flight turn, a delete's teardown
// drops the in-memory inbox entry so a serial worker can never re-dispatch a turn
// for a session that no longer exists.
func TestTeardown_NoLiveRun_RemovesInbox(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "a"}}}

	if err := s.teardownSessionRuntime(testWS("WS1"), "SES"); err != nil {
		t.Fatalf("teardown returned error: %v", err)
	}
	s.inbox.lock()
	_, ok := s.inbox.sessions[scopeKey("WS1", "SES")]
	s.inbox.unlock()
	if ok {
		t.Fatal("inbox entry must be removed after a successful teardown")
	}
}

// TestTeardown_OtherWorkspaceSameID_Untouched: session ids repeat across workspace
// stores, so deleting WS1/SES must leave WS2's own SES — its queue, and any turn
// running in it — completely alone.
func TestTeardown_OtherWorkspaceSameID_Untouched(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "a"}}}
	s.inbox.sessions[scopeKey("WS2", "SES")] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "b"}}}
	// A turn running in WS2's same-numbered session must not be seen (and cancelled)
	// by WS1's teardown — with a bare-id lookup it blocked the delete instead.
	s.runs.register("R1", "SES", "WS2", func() {})

	if err := s.teardownSessionRuntime(testWS("WS1"), "SES"); err != nil {
		t.Fatalf("teardown returned error: %v", err)
	}
	s.inbox.lock()
	_, gone := s.inbox.sessions[scopeKey("WS1", "SES")]
	other, kept := s.inbox.sessions[scopeKey("WS2", "SES")]
	s.inbox.unlock()
	if gone {
		t.Fatal("WS1's inbox entry must be removed")
	}
	if !kept || len(other.items) != 1 {
		t.Fatal("WS2's same-id inbox must survive another workspace's teardown")
	}
	if _, live := s.runs.sessionRunInfo("WS2", "SES"); !live {
		t.Fatal("WS2's run must still be registered")
	}
}

// TestTeardown_RequiresWorkspace: without a workspace the session cannot be
// identified (ids repeat across stores), so teardown refuses instead of tearing
// down whatever session happens to carry that id.
func TestTeardown_RequiresWorkspace(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	if err := s.teardownSessionRuntime(nil, "SES"); err == nil {
		t.Fatal("want an error when no workspace is given")
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
	if err := s.stopInflightTurn("WS", "SES", 2*time.Second); err != nil {
		t.Fatalf("want stopped, got error: %v", err)
	}
}

// TestStopInflightTurn_Timeout: a turn that ignores cancellation (never unregisters)
// is reported un-stoppable, which is exactly what makes the caller ABORT the delete
// instead of stranding the process.
func TestStopInflightTurn_Timeout(t *testing.T) {
	s := &Server{runs: newChatRuns()}
	s.runs.register("R1", "SES", "WS", func() {})
	if err := s.stopInflightTurn("WS", "SES", 50*time.Millisecond); err == nil {
		t.Fatal("want a timeout error for a turn that never stops")
	}
}

// TestKickInbox_FrozenDoesNotDispatch: while a teardown holds the closing freeze, the
// serial worker must not start — otherwise it would spawn a fresh turn (and subprocess)
// for a session mid-delete.
func TestKickInbox_FrozenDoesNotDispatch(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}, items: []inboxItem{{ClientMsgID: "a"}}, closing: true}

	s.kickInbox("WS1", "SES")
	s.inbox.lock()
	running := s.inbox.sessions[scopeKey("WS1", "SES")].running
	s.inbox.unlock()
	if running {
		t.Fatal("a frozen (closing) inbox must not start a worker")
	}
}

// TestResumeInboxAfterAbortedTeardown_Unfreezes: when a delete aborts, the freeze is
// lifted so the preserved queue can drain again (session lives on).
func TestResumeInboxAfterAbortedTeardown_Unfreezes(t *testing.T) {
	s := &Server{inbox: newInboxStore()}
	s.inbox.sessions[scopeKey("WS1", "SES")] = &sessionInbox{seen: map[string]bool{}, closing: true} // no items → kick is a no-op

	s.resumeInboxAfterAbortedTeardown("WS1", "SES")
	s.inbox.lock()
	closing := s.inbox.sessions[scopeKey("WS1", "SES")].closing
	s.inbox.unlock()
	if closing {
		t.Fatal("resume must clear the closing freeze")
	}
}
