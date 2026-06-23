package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/providers"
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

// TestMarkHiddenKeepsActivatableButOutOfBlock verifies hidden-lazy tools are
// excluded from the rendered block (VisibleLazyCatalog) yet remain in the full
// LazyCatalog (so activate_tools/tool_search still reach them), and that
// HiddenLazyCount reflects the hidden set.
func TestMarkHiddenKeepsActivatableButOutOfBlock(t *testing.T) {
	reg := NewRegistry(
		stubTool{name: "eager_a", desc: "always on"},
		stubTool{name: "lazy_visible", desc: "shown in block"},
		stubTool{name: "create_agent", desc: "self-mgmt"},
		stubTool{name: "create_flow", desc: "self-mgmt"},
	)
	reg.MarkLazy("lazy_visible")
	reg.MarkHidden("create_agent", "create_flow")

	// Full lazy catalog (activate/search source) includes hidden tools.
	full := names(reg.LazyCatalog(nil))
	if !eq(full, []string{"create_agent", "create_flow", "lazy_visible"}) {
		t.Fatalf("LazyCatalog must include hidden tools, got %v", full)
	}
	// Visible catalog (rendered block) excludes hidden tools.
	vis := names(reg.VisibleLazyCatalog(nil))
	if !eq(vis, []string{"lazy_visible"}) {
		t.Fatalf("VisibleLazyCatalog must exclude hidden tools, got %v", vis)
	}
	if n := reg.HiddenLazyCount(nil); n != 2 {
		t.Fatalf("HiddenLazyCount = %d, want 2", n)
	}
	// Hidden tools are still activatable (lazy → shippable once active).
	got := names(reg.ActiveDefs(nil, map[string]bool{"create_agent": true}))
	if !contains(got, "create_agent") {
		t.Fatalf("activated hidden tool must ship its schema, got %v", got)
	}
	// allow filter applies to the hidden count too.
	allow := func(n string) bool { return n != "create_flow" }
	if n := reg.HiddenLazyCount(allow); n != 1 {
		t.Fatalf("filtered HiddenLazyCount = %d, want 1", n)
	}
}

// TestBridgeableDefsExcludesCLINative verifies the CLI Interaction MCP bridge
// skips lazy built-ins that are CLI-native (WebFetch) or native-loop-context-bound
// (run_subagent), while still bridging an ordinary lazy self-management tool.
func TestBridgeableDefsExcludesCLINative(t *testing.T) {
	reg := NewRegistry(
		stubTool{name: "eager_a", desc: "always on"},
		stubTool{name: "create_agent", desc: "self-mgmt"},
		stubTool{name: "WebFetch", desc: "cli has WebFetch"},
		stubTool{name: "run_subagent", desc: "needs native run-agent ctx"},
	)
	reg.MarkLazy("create_agent", "WebFetch", "run_subagent")

	got := names(reg.BridgeableDefs(nil))
	if !eq(got, []string{"create_agent"}) {
		t.Errorf("BridgeableDefs = %v, want [create_agent] (WebFetch/run_subagent excluded)", got)
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

// exampledStub is a stub tool that carries input_examples.
type exampledStub struct{}

func (exampledStub) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "ex_tool",
		Description: "stub with examples",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`),
		Examples:    []json.RawMessage{json.RawMessage(`{"x":"hello"}`)},
	}
}
func (exampledStub) Call(context.Context, json.RawMessage) (string, error) { return "ok", nil }

// TestExamplesFoldIntoSchemaNotCatalog verifies input_examples are merged into the
// shipped InputSchema (active path) but never leak into the lightweight lazy
// catalog (name+description only).
func TestExamplesFoldIntoSchemaNotCatalog(t *testing.T) {
	reg := NewRegistry(exampledStub{})

	// Active/full schema carries an "examples" array with the sample.
	defs := reg.ActiveDefs(nil, nil)
	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(defs[0].InputSchema, &schema); err != nil {
		t.Fatalf("shipped schema not valid JSON: %v", err)
	}
	if _, ok := schema["examples"]; !ok {
		t.Errorf("shipped schema must contain folded examples, got: %s", defs[0].InputSchema)
	}
	if !strings.Contains(string(defs[0].InputSchema), `"hello"`) {
		t.Errorf("folded example value missing: %s", defs[0].InputSchema)
	}

	// Lazy catalog (mark it lazy) carries neither schema nor examples.
	reg.MarkLazy("ex_tool")
	cat := reg.LazyCatalog(nil)
	if len(cat) != 1 || len(cat[0].InputSchema) != 0 || len(cat[0].Examples) != 0 {
		t.Errorf("lazy catalog must omit schema+examples, got %+v", cat)
	}
}

// TestPilotToolExamplesAreValid checks every tool that ships input_examples: each
// example is a valid JSON object, and any field that is itself a STRINGIFIED JSON
// payload (flow graph, mcp args/env) parses as valid JSON too.
func TestPilotToolExamplesAreValid(t *testing.T) {
	// check validates examples; stringFields names properties whose value must be a
	// JSON string containing valid JSON (the escaped-JSON-string convention).
	check := func(name string, defs []json.RawMessage, stringFields ...string) {
		if len(defs) == 0 {
			t.Fatalf("%s has no examples", name)
		}
		for i, ex := range defs {
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(ex, &obj); err != nil {
				t.Errorf("%s example %d invalid JSON: %v", name, i, err)
				continue
			}
			for _, f := range stringFields {
				raw, ok := obj[f]
				if !ok {
					continue
				}
				var s string
				if err := json.Unmarshal(raw, &s); err != nil {
					t.Errorf("%s example %d field %q not a JSON string: %v", name, i, f, err)
				} else if !json.Valid([]byte(s)) {
					t.Errorf("%s example %d field %q is not valid JSON: %s", name, i, f, s)
				}
			}
		}
	}
	check("create_schedule", CreateScheduleTool{}.Def().Examples)
	check("create_flow", CreateFlowTool{}.Def().Examples, "graph")
	check("create_hook", CreateHookTool{}.Def().Examples)
	check("update_settings", UpdateSettingsTool{}.Def().Examples)
	check("create_mcp_server", CreateMCPServerTool{}.Def().Examples, "args", "env")
	check("create_agent", CreateAgentTool{}.Def().Examples)
	check("update_flow", UpdateFlowTool{}.Def().Examples, "graph")
	check("update_schedule", UpdateScheduleTool{}.Def().Examples)
	check("update_task", UpdateTaskTool{}.Def().Examples, "dependencies")
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
