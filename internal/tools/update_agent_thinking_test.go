package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// newThinkingAgent creates an agent through create_agent and returns its id, so
// the update tests start from a row whose ThinkingLevel is already resolved.
func newThinkingAgent(t *testing.T, d *db.DB, args string) string {
	t.Helper()
	out, err := NewCreateAgentTool(d, "actor", nil, nil, nil).Call(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("create_agent(%s): %v", args, err)
	}
	var res struct{ ID string }
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return res.ID
}

// TestUpdateAgentToolThinkingLevel pins the patch semantics: an omitted level
// leaves the stored tier alone, an explicit valid level is written.
func TestUpdateAgentToolThinkingLevel(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	id := newThinkingAgent(t, d, `{"name":"Patchable","provider":"anthropic","model":"claude-sonnet-5","thinkingLevel":"medium"}`)
	update := NewUpdateAgentTool(d, "actor", nil)

	// A patch that does not mention the field must not touch it.
	if _, err := update.Call(ctx, json.RawMessage(`{"id":"`+id+`","identity":"renamed"}`)); err != nil {
		t.Fatalf("update without thinkingLevel: %v", err)
	}
	got, err := d.GetAgent(ctx, id)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.ThinkingLevel != "medium" {
		t.Fatalf("omitted level changed the stored tier: got %q, want %q", got.ThinkingLevel, "medium")
	}

	// An explicit, model-supported level is stored verbatim.
	if _, err := update.Call(ctx, json.RawMessage(`{"id":"`+id+`","thinkingLevel":"high"}`)); err != nil {
		t.Fatalf("update with thinkingLevel: %v", err)
	}
	got, err = d.GetAgent(ctx, id)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.ThinkingLevel != "high" {
		t.Fatalf("explicit level: got %q, want %q", got.ThinkingLevel, "high")
	}
}

// TestUpdateAgentToolRejectsBadThinkingLevel covers the three ways a patch can
// name an illegal tier: an unknown token, the empty string (no longer a valid
// stored value), and a tier the target model cannot use.
func TestUpdateAgentToolRejectsBadThinkingLevel(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	update := NewUpdateAgentTool(d, "actor", nil)

	id := newThinkingAgent(t, d, `{"name":"Strict","provider":"anthropic","model":"claude-sonnet-5","thinkingLevel":"medium"}`)
	for _, tc := range []struct{ name, level string }{
		{"unknown token", `"turbo"`},
		{"empty string", `""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := update.Call(ctx, json.RawMessage(`{"id":"`+id+`","thinkingLevel":`+tc.level+`}`))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "thinkingLevel") {
				t.Fatalf("error should name the field, got: %v", err)
			}
			got, gerr := d.GetAgent(ctx, id)
			if gerr != nil {
				t.Fatalf("get agent: %v", gerr)
			}
			if got.ThinkingLevel != "medium" {
				t.Fatalf("rejected patch still wrote %q", got.ThinkingLevel)
			}
		})
	}

	// deepseek-flash has no extended-reasoning mode at all (non-thinking class),
	// so every tier but "off" is a silent no-op and must be refused.
	t.Run("model class", func(t *testing.T) {
		flash := newThinkingAgent(t, d, `{"name":"Flash","provider":"deepseek","model":"deepseek-flash","thinkingLevel":"off"}`)
		_, err := update.Call(ctx, json.RawMessage(`{"id":"`+flash+`","thinkingLevel":"high"}`))
		if err == nil {
			t.Fatal("expected an error for high on a non-thinking model")
		}
		if !strings.Contains(err.Error(), "deepseek-flash") {
			t.Fatalf("error should name the model, got: %v", err)
		}
	})
}
