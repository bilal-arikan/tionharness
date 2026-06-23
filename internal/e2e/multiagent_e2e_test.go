package e2e

import (
	"context"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// TestMultiAgent_SequentialRepliesShareHistory drives one turn answered by two
// agents in order (the "@mention" routing the chat layer does): the user message
// is persisted once, both agents reply, and the SECOND agent sees the first
// agent's reply in the history it is given — so a panel of agents builds on each
// other within a single turn.
func TestMultiAgent_SequentialRepliesShareHistory(t *testing.T) {
	prov := newScriptedProvider(
		sayText("Ada: I propose we cache the catalog."),
		sayText("Bryn: Building on Ada, I'd add a TTL."),
	)
	h := newHarness(t, prov)
	ada := h.newAgent("Ada")
	bryn := h.newAgent("Bryn")
	sess := h.newSession(ada)

	results := h.sendMulti([]db.Agent{ada, bryn}, sess, "How should we speed up the catalog?")
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].resp.Text != "Ada: I propose we cache the catalog." {
		t.Errorf("Ada reply = %q", results[0].resp.Text)
	}
	if results[1].resp.Text != "Bryn: Building on Ada, I'd add a TTL." {
		t.Errorf("Bryn reply = %q", results[1].resp.Text)
	}

	// Bryn's request (the second provider call) must carry the user prompt AND
	// Ada's reply — proof later agents see earlier ones within the turn.
	msgs := prov.lastReq.Messages
	if len(msgs) != 2 {
		t.Fatalf("Bryn saw %d messages, want 2 (user + Ada's reply): %+v", len(msgs), msgs)
	}
	if msgs[0].Role != providers.RoleUser || msgs[1].Role != providers.RoleAssistant {
		t.Fatalf("unexpected roles: %+v", msgs)
	}
	if msgs[1].Text != "Ada: I propose we cache the catalog." {
		t.Errorf("Bryn did not see Ada's reply, got %q", msgs[1].Text)
	}

	// All three messages persisted under one session: 1 user + 2 assistant.
	all, _ := h.db.ListMessages(context.Background(), sess.ID)
	if len(all) != 3 {
		t.Fatalf("persisted %d messages, want 3", len(all))
	}
	if all[1].AgentID != ada.ID || all[2].AgentID != bryn.ID {
		t.Errorf("replies attributed to wrong agents: %s, %s", all[1].AgentID, all[2].AgentID)
	}
}
