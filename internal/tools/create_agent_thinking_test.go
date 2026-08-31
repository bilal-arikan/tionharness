package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// createdAgentLevel runs create_agent with the given raw arguments and returns
// the ThinkingLevel actually persisted for the new agent.
func createdAgentLevel(t *testing.T, d *db.DB, actor, args string) string {
	t.Helper()
	ctx := context.Background()
	out, err := NewCreateAgentTool(d, actor, nil, nil, nil).Call(ctx, json.RawMessage(args))
	if err != nil {
		t.Fatalf("create_agent(%s): %v", args, err)
	}
	var res struct{ ID string }
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	got, err := d.GetAgent(ctx, res.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	return got.ThinkingLevel
}

// TestCreateAgentToolCarriesThinkingLevel pins that create_agent now decides the
// reasoning tier itself instead of dropping through db.CreateAgent's fallback:
// an explicit level is stored verbatim, an omitted one is resolved with
// db.LegacyThinkingLevelFor for the RESOLVED provider kind.
func TestCreateAgentToolCarriesThinkingLevel(t *testing.T) {
	d := openTestDB(t)

	if got := createdAgentLevel(t, d, "actor", `{"name":"Explicit","provider":"anthropic","thinkingLevel":"medium"}`); got != "medium" {
		t.Fatalf("explicit level: got %q, want %q", got, "medium")
	}
	// claude-cli is a CLI provider: the legacy rule maps a missing level to high.
	if got := createdAgentLevel(t, d, "actor", `{"name":"CliDefault","provider":"claude-cli"}`); got != db.LegacyThinkingLevelFor("claude-cli") {
		t.Fatalf("omitted level on claude-cli: got %q, want %q", got, db.LegacyThinkingLevelFor("claude-cli"))
	}
	// Any other provider historically ran with reasoning off.
	if got := createdAgentLevel(t, d, "actor", `{"name":"NativeDefault","provider":"anthropic"}`); got != db.LegacyThinkingLevelFor("anthropic") {
		t.Fatalf("omitted level on anthropic: got %q, want %q", got, db.LegacyThinkingLevelFor("anthropic"))
	}
}

// TestCreateAgentToolRejectsUnknownThinkingLevel verifies a bad tier fails the
// call loudly rather than being stored or silently swallowed.
func TestCreateAgentToolRejectsUnknownThinkingLevel(t *testing.T) {
	d := openTestDB(t)
	_, err := NewCreateAgentTool(d, "actor", nil, nil, nil).
		Call(context.Background(), json.RawMessage(`{"name":"Bad","provider":"anthropic","thinkingLevel":"turbo"}`))
	if err == nil {
		t.Fatal("expected an error for an unknown thinkingLevel")
	}
	if !strings.Contains(err.Error(), "thinkingLevel") {
		t.Fatalf("error should name the field, got: %v", err)
	}
}
