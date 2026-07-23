package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// TestActivateToleratesNamespaceConfusion: the model naming a tool with an
// invented mcp__server__ prefix still activates the right tool when the bare name
// is unambiguous — instead of a dead-end "unknown" rejection.
func TestActivateToleratesNamespaceConfusion(t *testing.T) {
	catalog := []providers.ToolDef{
		{Name: "get_flow", Description: "get a flow"},
		{Name: "list_flows", Description: "list flows"},
	}
	tool := NewActivateToolsTool(NewActiveTools(), catalog)

	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{"mcp__tionswarm_extended__get_flow"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Activated") || !strings.Contains(out, "get_flow") {
		t.Fatalf("namespace-prefixed name should resolve to get_flow:\n%s", out)
	}
	if strings.Contains(out, "Unknown") {
		t.Fatalf("should not report unknown:\n%s", out)
	}
}

// TestActivateExactStillWins + ambiguity stays unknown (never mis-routed).
func TestActivateAmbiguousStaysUnknown(t *testing.T) {
	catalog := []providers.ToolDef{
		{Name: "alpha__run", Description: "a"},
		{Name: "beta__run", Description: "b"},
	}
	tool := NewActivateToolsTool(NewActiveTools(), catalog)

	// Exact name still activates.
	if out, _ := tool.Call(context.Background(), mustJSON(t, map[string]any{"names": []string{"alpha__run"}})); !strings.Contains(out, "Activated") {
		t.Fatalf("exact name must activate:\n%s", out)
	}
	// A bare "run" is ambiguous across two servers → must NOT be silently routed.
	out, _ := tool.Call(context.Background(), mustJSON(t, map[string]any{"names": []string{"run"}}))
	if !strings.Contains(out, "Unknown") {
		t.Fatalf("ambiguous bare name must stay unknown, not mis-route:\n%s", out)
	}
}
