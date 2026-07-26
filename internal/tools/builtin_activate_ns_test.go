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
	tool := NewActivateToolsTool(NewActiveTools(), catalog, nil)

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
	tool := NewActivateToolsTool(NewActiveTools(), catalog, nil)

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

// TestActivateAlreadyAvailableTool: activating an always-on (eager) tool that is
// NOT in the lazy catalog (e.g. insight_scan) must return a helpful "already
// available" note rather than the misleading "unknown name" — and must NOT push
// it into the active set (it is already shipped every turn).
func TestActivateAlreadyAvailableTool(t *testing.T) {
	catalog := []providers.ToolDef{{Name: "lazy_a", Description: "A"}}
	eager := map[string]bool{"insight_scan": true, "insight_list_findings": true}
	active := NewActiveTools()
	tool := NewActivateToolsTool(active, catalog, eager)

	// Exact eager name → "already available", not unknown, not activated.
	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"names": []string{"insight_scan"}}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "already available") {
		t.Fatalf("eager tool should report already available:\n%s", out)
	}
	if strings.Contains(out, "Unknown") {
		t.Fatalf("eager tool must not be reported unknown:\n%s", out)
	}
	if active.Has("insight_scan") {
		t.Fatalf("eager tool must not enter the active set")
	}

	// Namespace-prefixed eager name still resolves to the always-available note.
	out, _ = tool.Call(context.Background(), mustJSON(t, map[string]any{"names": []string{"mcp__x__insight_list_findings"}}))
	if !strings.Contains(out, "already available") || strings.Contains(out, "Unknown") {
		t.Fatalf("prefixed eager name should report already available:\n%s", out)
	}

	// Mixed: one lazy (activates) + one eager (noted) in a single call.
	out, _ = tool.Call(context.Background(), mustJSON(t, map[string]any{"names": []string{"lazy_a", "insight_scan"}}))
	if !strings.Contains(out, "Activated") || !strings.Contains(strings.ToLower(out), "already available") {
		t.Fatalf("mixed call should both activate lazy_a and note insight_scan:\n%s", out)
	}
	if !active.Has("lazy_a") {
		t.Fatalf("lazy_a should have activated in the mixed call")
	}
}
