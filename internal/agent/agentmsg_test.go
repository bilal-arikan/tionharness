package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
)

func TestResolveThinkingBudget(t *testing.T) {
	// Classic models: unchanged (off stays off, levels map as before).
	if got := resolveThinkingBudget("claude-opus-4-8", "off"); got != 0 {
		t.Errorf("classic off = %d, want 0", got)
	}
	if got := resolveThinkingBudget("claude-opus-4-8", "high"); got != 16384 {
		t.Errorf("classic high = %d, want 16384", got)
	}
	// Adaptive-only models: off is floored to the minimal adaptive budget.
	if got := resolveThinkingBudget("claude-fable-5", "off"); got != 1024 {
		t.Errorf("fable off = %d, want 1024 (min adaptive)", got)
	}
	// Adaptive model with an explicit higher level keeps the higher budget.
	if got := resolveThinkingBudget("claude-fable-5", "high"); got != 16384 {
		t.Errorf("fable high = %d, want 16384", got)
	}
}

func TestSendAgentMessageDelivers(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	from, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Sender", Provider: "anthropic"})
	to, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Receiver", Provider: "anthropic"})

	// Deliver by display name.
	res, err := rt.SendAgentMessage(ctx, from.ID, "Receiver", "please review PR-42")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.AgentName != "Receiver" {
		t.Fatalf("resolved name = %q, want Receiver", res.AgentName)
	}

	// The message landed in the recipient's inbox session, attributed to sender.
	msgs, err := rt.db.ListMessages(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 inbox message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("inbox message role = %q, want user", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Text, "Sender") || !strings.Contains(msgs[0].Text, "PR-42") {
		t.Errorf("inbox message missing attribution/body: %q", msgs[0].Text)
	}

	// A second message reuses the same stable inbox thread (no duplicate session).
	res2, err := rt.SendAgentMessage(ctx, from.ID, to.ID, "ping again")
	if err != nil {
		t.Fatalf("second send: %v", err)
	}
	if res2.SessionID != res.SessionID {
		t.Fatalf("inbox session not reused: %s vs %s", res2.SessionID, res.SessionID)
	}

	// Unknown recipient → clear error the model can react to.
	if _, err := rt.SendAgentMessage(ctx, from.ID, "Nobody", "hi"); err == nil {
		t.Fatal("expected error for unknown recipient")
	}
}
