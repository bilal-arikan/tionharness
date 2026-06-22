package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

// TestFormatAgentMessage checks the sender-identity tag, with and without summary.
func TestFormatAgentMessage(t *testing.T) {
	got := formatAgentMessage("Ada", "task 1", "start now")
	if !strings.Contains(got, `from="Ada"`) || !strings.Contains(got, `summary="task 1"`) || !strings.Contains(got, "start now") {
		t.Fatalf("tag missing parts: %q", got)
	}
	noSum := formatAgentMessage("Ada", "", "hi")
	if strings.Contains(noSum, "summary=") {
		t.Errorf("empty summary should be omitted: %q", noSum)
	}
}

// TestDeliverAgentMessage_Synchronous verifies the synchronous half of a peer DM:
// the sender-tagged message is appended to the RECIPIENT's persistent inbox
// session. (The background turn fails — no provider — but that is async and not
// asserted, mirroring the spawn tests.)
func TestDeliverAgentMessage_Synchronous(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	sender, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "anthropic", Model: "m"})
	target, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Kai", Provider: "anthropic", Model: "m"})

	res, err := rt.DeliverAgentMessage(ctx, sender.ID, "Kai", "1. gorev", "task #1 basla")
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if !strings.Contains(res, "Kai") {
		t.Errorf("confirmation should name the recipient: %q", res)
	}

	inbox, err := rt.db.GetOrCreateKindSession(ctx, target.ID, inboxSessionKind, "📥 Inbox")
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	msgs, err := rt.db.ListMessages(ctx, inbox.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("inbox should have the delivered message")
	}
	first := msgs[0]
	if first.Role != "user" {
		t.Errorf("delivered message should be a user turn, got %q", first.Role)
	}
	if !strings.Contains(first.Text, `from="Ada"`) || !strings.Contains(first.Text, "task #1") {
		t.Errorf("delivered message missing sender tag/body: %q", first.Text)
	}
}

// TestDeliverAgentMessage_RejectsSelf guards against an agent messaging itself.
func TestDeliverAgentMessage_RejectsSelf(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Solo", Provider: "anthropic", Model: "m"})
	if _, err := rt.DeliverAgentMessage(ctx, a.ID, "Solo", "", "hey me"); err == nil {
		t.Fatal("expected an error when messaging yourself")
	}
}

// TestDeliverAgentMessage_RejectsEmpty guards the message precondition.
func TestDeliverAgentMessage_RejectsEmpty(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic", Model: "m"})
	rt.db.CreateAgent(ctx, db.Agent{Name: "B", Provider: "anthropic", Model: "m"})
	a, _ := rt.resolveAgent(ctx, "A")
	if _, err := rt.DeliverAgentMessage(ctx, a.ID, "B", "", "   "); err == nil {
		t.Fatal("expected an error for an empty message")
	}
}
