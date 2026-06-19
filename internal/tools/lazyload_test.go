package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal/swarmgo/internal/providers"
)

// stubTool is a minimal Tool for registry tests.
type stubTool struct {
	name string
	desc string
}

func (s stubTool) Def() providers.ToolDef {
	return providers.ToolDef{Name: s.name, Description: s.desc, InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (stubTool) Call(context.Context, json.RawMessage) (string, error) { return "ok", nil }

func TestRegistryActiveDefsAndLazyCatalog(t *testing.T) {
	reg := NewRegistry(
		stubTool{name: "eager_a", desc: "always on"},
		stubTool{name: "lazy_a", desc: "on demand A"},
		stubTool{name: "lazy_b", desc: "on demand B"},
	)
	reg.MarkLazy("lazy_a", "lazy_b")

	// At turn start (no active): only eager tools ship.
	got := names(reg.ActiveDefs(nil, nil))
	if !eq(got, []string{"eager_a"}) {
		t.Errorf("eager-only ActiveDefs = %v", got)
	}

	// After activating lazy_a: it joins the shipped set.
	got = names(reg.ActiveDefs(nil, map[string]bool{"lazy_a": true}))
	if !eq(got, []string{"eager_a", "lazy_a"}) {
		t.Errorf("active ActiveDefs = %v", got)
	}

	// Lazy catalog lists only the lazy tools (name + desc, no schema).
	cat := reg.LazyCatalog(nil)
	if !eq(names(cat), []string{"lazy_a", "lazy_b"}) {
		t.Errorf("LazyCatalog = %v", names(cat))
	}
	for _, d := range cat {
		if len(d.InputSchema) != 0 {
			t.Errorf("LazyCatalog should omit schema for %s", d.Name)
		}
	}

	// allow filter applies to both.
	allow := func(n string) bool { return n != "lazy_b" }
	if got := names(reg.LazyCatalog(allow)); !eq(got, []string{"lazy_a"}) {
		t.Errorf("filtered LazyCatalog = %v", got)
	}
}

// TestBridgeableDefsExcludesCLINative verifies the CLI Interaction MCP bridge
// skips lazy built-ins that are CLI-native (http_get) or native-loop-context-bound
// (call_agent), while still bridging an ordinary lazy self-management tool.
func TestBridgeableDefsExcludesCLINative(t *testing.T) {
	reg := NewRegistry(
		stubTool{name: "eager_a", desc: "always on"},
		stubTool{name: "create_agent", desc: "self-mgmt"},
		stubTool{name: "http_get", desc: "cli has WebFetch"},
		stubTool{name: "call_agent", desc: "needs native delegation ctx"},
	)
	reg.MarkLazy("create_agent", "http_get", "call_agent")

	got := names(reg.BridgeableDefs(nil))
	if !eq(got, []string{"create_agent"}) {
		t.Errorf("BridgeableDefs = %v, want [create_agent] (http_get/call_agent excluded)", got)
	}
	// Schemas are full (not stripped) for bridged tools.
	for _, d := range reg.BridgeableDefs(nil) {
		if len(d.InputSchema) == 0 {
			t.Errorf("BridgeableDefs must ship full schema for %s", d.Name)
		}
	}
}

func TestActiveToolsPrune(t *testing.T) {
	a := NewActiveTools()
	a.SetIter(0)
	a.Activate("x", "y")
	a.SetIter(1)
	a.MarkUsed("x") // x used at iter 1; y idle since iter 0
	a.SetIter(5)
	pruned := a.Prune(3) // y idle 5 iterations > 3 → dropped; x used at 1, idle 4 → also dropped
	if !contains(pruned, "y") {
		t.Errorf("expected y pruned, got %v", pruned)
	}
	// Re-test with a fresh set where x stays fresh.
	b := NewActiveTools()
	b.SetIter(0)
	b.Activate("x")
	b.SetIter(2)
	b.MarkUsed("x")
	b.SetIter(3)
	if p := b.Prune(3); len(p) != 0 {
		t.Errorf("fresh tool should not be pruned, got %v", p)
	}
}

func TestActivateToolsTool(t *testing.T) {
	active := NewActiveTools()
	cat := []providers.ToolDef{{Name: "lazy_a", Description: "A"}, {Name: "lazy_b", Description: "B"}}
	tool := NewActivateToolsTool(active, cat)

	out, err := tool.Call(context.Background(), json.RawMessage(`{"names":["lazy_a","nope"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !active.Has("lazy_a") {
		t.Errorf("lazy_a should be active")
	}
	if active.Has("nope") {
		t.Errorf("unknown tool must not activate")
	}
	if !strings.Contains(out, "lazy_a") || !strings.Contains(out, "Unknown") {
		t.Errorf("activate output = %q", out)
	}

	// tool_search keyword search.
	ts := NewToolSearchTool(cat)
	fout, _ := ts.Call(context.Background(), json.RawMessage(`{"query":"B"}`))
	if !strings.Contains(fout, "lazy_b") || strings.Contains(fout, "lazy_a") {
		t.Errorf("tool_search output = %q", fout)
	}
	if got := ts.Def().Name; got != "tool_search" {
		t.Errorf("tool name = %q, want tool_search", got)
	}
}

// ---- helpers ----

func names(defs []providers.ToolDef) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
