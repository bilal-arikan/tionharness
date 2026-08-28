package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestActivateRedirectsExternalMCPName: on the native path too, an EXTERNAL
// mcp__<server>__ name is not "unknown" — it is served by a different loader. Before
// this redirect the model got a dead-end rejection and retried the same call
// (WS20/SES79).
func TestActivateRedirectsExternalMCPName(t *testing.T) {
	catalog := []providers.ToolDef{{Name: "lazy_a", Description: "A"}}
	tool := NewActivateToolsTool(NewActiveTools(), catalog, nil)

	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{"mcp__codebase-memory-mcp__search_graph"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ToolSearch") {
		t.Fatalf("external MCP name must be redirected to ToolSearch:\n%s", out)
	}
	if !strings.Contains(out, "select:mcp__codebase-memory-mcp__search_graph") {
		t.Fatalf("redirect must spell out the select: query:\n%s", out)
	}
}

// TestToolSearchRedirectsSelectQuery: the native tool_search must answer a `select:`
// query over a TionHarness namespace with the activate_tools pointer instead of the
// empty result that produced the three-turn retry loop.
func TestToolSearchRedirectsSelectQuery(t *testing.T) {
	tool := NewToolSearchTool([]providers.ToolDef{{Name: "lazy_a", Description: "A"}})

	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"query": "select:mcp__tionharness_extended__list_tasks",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("tool_search must not return an empty result for a select: query")
	}
	if !strings.Contains(out, "activate_tools") {
		t.Fatalf("tool_search must redirect to activate_tools:\n%s", out)
	}
}
