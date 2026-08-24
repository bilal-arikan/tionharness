package e2e

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestConversation_MultiTurnHistory drives a two-turn conversation end to end and
// proves the turn pipeline persists every message and replays the growing history
// back to the model: the second turn's request must carry the first exchange.
func TestConversation_MultiTurnHistory(t *testing.T) {
	prov := newScriptedProvider(
		sayText("Hello Bilal, good to meet you."),
		sayText("Earlier you greeted me."),
	)
	h := newHarness(t, prov)
	ag := h.newAgent("Ada")
	sess := h.newSession(ag)

	r1 := h.send(ag, sess, "Hi, I'm Bilal.")
	if r1.resp.Text != "Hello Bilal, good to meet you." {
		t.Fatalf("turn 1 reply = %q", r1.resp.Text)
	}

	r2 := h.send(ag, sess, "What did I do earlier?")
	if r2.resp.Text != "Earlier you greeted me." {
		t.Fatalf("turn 2 reply = %q", r2.resp.Text)
	}

	// The second turn (no tools) is a single provider call, so lastReq is its
	// request: it must contain user1, assistant1, user2 — proof the history was
	// replayed, not just the latest message.
	msgs := prov.lastReq.Messages
	if len(msgs) != 3 {
		t.Fatalf("turn 2 sent %d messages, want 3 (user1, assistant1, user2): %+v", len(msgs), msgs)
	}
	if msgs[0].Role != providers.RoleUser || msgs[0].Text != "Hi, I'm Bilal." {
		t.Errorf("msg[0] = %+v, want user 'Hi, I'm Bilal.'", msgs[0])
	}
	if msgs[1].Role != providers.RoleAssistant || msgs[1].Text != "Hello Bilal, good to meet you." {
		t.Errorf("msg[1] = %+v, want assistant turn-1 reply", msgs[1])
	}
	if msgs[2].Role != providers.RoleUser || msgs[2].Text != "What did I do earlier?" {
		t.Errorf("msg[2] = %+v, want user turn-2 prompt", msgs[2])
	}

	// All four messages (2 user + 2 assistant) are durably persisted.
	all, err := h.db.ListMessages(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("persisted %d messages, want 4", len(all))
	}

	// The session's running message count tracks the appended turns.
	got, _ := h.db.GetSession(context.Background(), sess.ID)
	if got.MessageCount != 4 {
		t.Errorf("session MessageCount = %d, want 4", got.MessageCount)
	}
}
