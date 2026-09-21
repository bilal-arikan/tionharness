package api

import (
	"sync"
	"testing"
)

// queueIDs reads the session's WAITING queue in order.
func queueIDs(s *Server, wsID, sessionID string) []string {
	s.inbox.lock()
	defer s.inbox.unlock()
	ib := s.inbox.at(wsID, sessionID)
	if ib == nil {
		return nil
	}
	ids := make([]string, 0, len(ib.items))
	for _, it := range ib.items {
		ids = append(ids, it.ClientMsgID)
	}
	return ids
}

// The whole promise of the interrupt gesture: the user's message runs NEXT. With
// the old two-call shape (stop, then send) a message already waiting in the queue
// stayed ahead of it, so the interrupt ran second.
func TestInterruptMessageJumpsAheadOfWaitingQueue(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	freezeQueue(t, s, "WS1", "SES1")
	if !s.enqueueMessage("WS1", chatReq{SessionID: "SES1", Message: "önceden bekleyen"}, "waiting") {
		t.Fatal("seed enqueue rejected")
	}

	if !s.enqueueMessageAtHead("WS1", chatReq{SessionID: "SES1", Message: "kes ve bunu yap"}, "interrupt") {
		t.Fatal("interrupt enqueue rejected")
	}

	got := queueIDs(s, "WS1", "SES1")
	want := []string{"interrupt", "waiting"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("queue order = %v, want %v", got, want)
	}
}

// freezeQueue stops the serial worker from draining the session's queue, so a test
// can assert the queue's SHAPE without racing a real dispatch. This is the same
// latch session teardown uses (session_teardown.go), not a test-only backdoor.
func freezeQueue(t *testing.T, s *Server, wsID, sessionID string) {
	t.Helper()
	s.inbox.lock()
	defer s.inbox.unlock()
	ib := s.inbox.at(wsID, sessionID)
	if ib == nil {
		ib = &sessionInbox{seen: make(map[string]bool), wsID: wsID}
		s.inbox.sessions[scopeKey(wsID, sessionID)] = ib
	}
	ib.closing = true
}

// The race the atomic action closes: messages arriving CONCURRENTLY with the
// interrupt must not end up in front of it. enqueueMessageAtHead inserts under ONE
// lock hold, so no matter how the concurrent enqueues interleave, the interrupt is
// the head afterwards and nothing is lost or duplicated along the way.
func TestInterruptHeadInsertIsAtomicUnderConcurrentEnqueue(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	// Without this the worker kicked by each enqueue drains the queue as it is
	// being built, and the assertion below would be testing dispatch speed rather
	// than insert ordering.
	freezeQueue(t, s, "WS1", "SES1")

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.enqueueMessage("WS1", chatReq{SessionID: "SES1", Message: "arka plan"}, string(rune('a'+n)))
		}(i)
	}
	wg.Wait()

	if !s.enqueueMessageAtHead("WS1", chatReq{SessionID: "SES1", Message: "kes"}, "interrupt") {
		t.Fatal("interrupt enqueue rejected")
	}

	got := queueIDs(s, "WS1", "SES1")
	if len(got) != 9 {
		t.Fatalf("queue length = %d, want 9 (nothing lost or duplicated)", len(got))
	}
	if got[0] != "interrupt" {
		t.Fatalf("head = %q, want the interrupt message", got[0])
	}
}

// A retried interrupt (same clientMsgId) must not enqueue the text twice — the
// same replay guard a normal send gets.
func TestInterruptHeadInsertIsIdempotentOnClientMsgID(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	freezeQueue(t, s, "WS1", "SES1")

	if !s.enqueueMessageAtHead("WS1", chatReq{SessionID: "SES1", Message: "kes"}, "dup") {
		t.Fatal("first interrupt rejected")
	}
	if s.enqueueMessageAtHead("WS1", chatReq{SessionID: "SES1", Message: "kes"}, "dup") {
		t.Fatal("replayed interrupt must be rejected, not enqueued again")
	}

	if got := queueIDs(s, "WS1", "SES1"); len(got) != 1 {
		t.Fatalf("queue = %v, want exactly one item", got)
	}
}

// The interrupt message becomes a normal turn, so the per-turn settings the
// composer had selected must survive the trip. Dropping them silently downgraded
// the turn (agent's default thinking/permission instead of the user's choice).
func TestInterruptCarriesTurnSettings(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	freezeQueue(t, s, "WS1", "SES1")

	req := chatReq{
		SessionID:      "SES1",
		Message:        "kes",
		AgentIDs:       []string{"AGT7"},
		ThinkingLevel:  "high",
		PermissionMode: "ask",
	}
	if !s.enqueueMessageAtHead("WS1", req, "c-1") {
		t.Fatal("interrupt enqueue rejected")
	}

	s.inbox.lock()
	defer s.inbox.unlock()
	items := s.inbox.at("WS1", "SES1").items
	if len(items) != 1 {
		t.Fatalf("queue length = %d, want 1", len(items))
	}
	got := items[0].Req
	if got.ThinkingLevel != "high" || got.PermissionMode != "ask" {
		t.Fatalf("thinking/permission = %q/%q, want high/ask", got.ThinkingLevel, got.PermissionMode)
	}
	if len(got.AgentIDs) != 1 || got.AgentIDs[0] != "AGT7" {
		t.Fatalf("agentIDs = %v, want [AGT7]", got.AgentIDs)
	}
	// The resolved id is carried on the request too, so the worker can stamp it
	// onto the turn's terminal hub events.
	if got.ClientMsgID != "c-1" {
		t.Fatalf("clientMsgID = %q, want c-1", got.ClientMsgID)
	}
}
