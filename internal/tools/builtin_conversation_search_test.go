package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

func TestConversationSearchTool(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	agent, _ := d.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	s, _ := d.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "Deploy"})
	d.AddMessage(ctx, db.Message{SessionID: s.ID, Role: "user", Text: "deploy the gateway"})
	d.AddMessage(ctx, db.Message{SessionID: s.ID, Role: "assistant", Text: "deployed via docker", AgentID: agent.ID})

	tool := NewConversationSearchTool(d)

	// Match, default role.
	out, err := tool.Call(ctx, json.RawMessage(`{"query":"deploy"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "gateway") && !strings.Contains(out, "docker") {
		t.Fatalf("expected a match snippet, got %q", out)
	}

	// Role filter: exactly one hit, the assistant one.
	out, _ = tool.Call(ctx, json.RawMessage(`{"query":"deploy","role":"assistant"}`))
	if strings.Count(out, "- [") != 1 {
		t.Fatalf("role filter should give 1 hit, got %q", out)
	}
	if !strings.Contains(out, "[assistant]") || strings.Contains(out, "[user]") {
		t.Fatalf("expected only assistant hit, got %q", out)
	}

	// Full mode returns the whole matched message verbatim.
	out, _ = tool.Call(ctx, json.RawMessage(`{"query":"gateway","full":true}`))
	if !strings.Contains(out, "deploy the gateway") {
		t.Fatalf("full mode should return verbatim text, got %q", out)
	}

	// Context mode includes surrounding turns with the hit marked »».
	out, _ = tool.Call(ctx, json.RawMessage(`{"query":"gateway","context":1}`))
	if !strings.Contains(out, "»»") || !strings.Contains(out, "docker") {
		t.Fatalf("context mode should include neighbour turns + marker, got %q", out)
	}

	// session_id scoping: unknown session → no hits.
	out, _ = tool.Call(ctx, json.RawMessage(`{"query":"deploy","session_id":"NOPE"}`))
	if !strings.Contains(out, "No matching") {
		t.Fatalf("unknown session_id should yield no hits, got %q", out)
	}

	// No match.
	out, _ = tool.Call(ctx, json.RawMessage(`{"query":"nonexistentterm"}`))
	if !strings.Contains(out, "No matching") {
		t.Fatalf("expected no-match message, got %q", out)
	}

	// Empty query rejected.
	if _, err := tool.Call(ctx, json.RawMessage(`{"query":"  "}`)); err == nil {
		t.Fatalf("expected error for empty query")
	}
}
