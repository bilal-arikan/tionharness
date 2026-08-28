package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// bundleFixture builds an activate_tools over a small catalog with one built-in
// bundle (group:automation) and one MCP bundle (mcp:srv).
func bundleFixture(t *testing.T, active *ActiveTools) ActivateToolsTool {
	t.Helper()
	catalog := []providers.ToolDef{
		{Name: "get_flow", Description: "get a flow"},
		{Name: "list_flows", Description: "list flows"},
		{Name: "srv__alpha", Description: "alpha tool"},
		{Name: "srv__beta", Description: "beta tool"},
	}
	bundles := map[string][]string{
		GroupPrefix + CategoryAutomation: {"get_flow", "list_flows"},
		MCPBundlePrefix + "srv":          {"srv__alpha", "srv__beta"},
	}
	return NewActivateToolsToolBundled(active, catalog, nil, bundles)
}

// TestActivateBundleListsWithoutActivating is the contract test: opening a bundle
// returns member summaries and leaves the ACTIVE set — i.e. the shipped schema
// set — completely untouched.
func TestActivateBundleListsWithoutActivating(t *testing.T) {
	active := NewActiveTools()
	tool := bundleFixture(t, active)

	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{GroupPrefix + CategoryAutomation},
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"group:automation (2 tools)", "- get_flow — get a flow", "- list_flows — list flows"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in bundle listing:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Activated") {
		t.Fatalf("a bundle must not report activations:\n%s", out)
	}
	if got := active.Snapshot(); len(got) != 0 {
		t.Fatalf("bundle activation leaked into the active set: %v", got)
	}
	if !active.HasBundle(GroupPrefix + CategoryAutomation) {
		t.Fatal("the opened bundle must be recorded")
	}
	if got := active.OpenBundles(); len(got) != 1 || got[0] != GroupPrefix+CategoryAutomation {
		t.Fatalf("OpenBundles() = %v", got)
	}
}

// TestActivateMixedNamesAndBundle: names still activate normally when a bundle key
// travels in the same call, and only the names reach the active set.
func TestActivateMixedNamesAndBundle(t *testing.T) {
	active := NewActiveTools()
	tool := bundleFixture(t, active)

	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{"srv__alpha", MCPBundlePrefix + "srv"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Activated 1 tool(s)") || !strings.Contains(out, "mcp:srv (2 tools)") {
		t.Fatalf("expected both an activation and a listing:\n%s", out)
	}
	snap := active.Snapshot()
	if len(snap) != 1 || !snap["srv__alpha"] {
		t.Fatalf("only the named tool may be active, got %v", snap)
	}
}

// TestActivateUnknownBundleKey: a mistyped key reports the known bundles instead
// of falling into the fuzzy tool-name path.
func TestActivateUnknownBundleKey(t *testing.T) {
	tool := bundleFixture(t, NewActiveTools())

	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{"group:automaton", "group:config"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	// group:automaton is not a category; group:config is valid but has no member here.
	for _, want := range []string{"Unknown bundle(s): group:automaton, group:config", "Known bundles: group:automation, mcp:srv"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
}

// TestActivateBundleListLimit: a huge bundle is truncated at BundleListLimit and
// says how many members it hid.
func TestActivateBundleListLimit(t *testing.T) {
	const total = BundleListLimit + 7
	var catalog []providers.ToolDef
	var members []string
	for i := 0; i < total; i++ {
		name := fmt.Sprintf("srv__t%03d", i)
		catalog = append(catalog, providers.ToolDef{Name: name, Description: "d"})
		members = append(members, name)
	}
	tool := NewActivateToolsToolBundled(NewActiveTools(), catalog, nil, map[string][]string{
		MCPBundlePrefix + "srv": members,
	})

	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{MCPBundlePrefix + "srv"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(out, "\n- "); got != BundleListLimit {
		t.Fatalf("listed %d members, want %d", got, BundleListLimit)
	}
	if !strings.Contains(out, "…and 7 more not shown") {
		t.Fatalf("truncation notice missing:\n%s", out)
	}
}

// TestBundleKeysIgnoredWithoutIndex: the un-bundled constructor keeps its old
// behavior — a bundle key is simply unknown, never a fuzzy name match.
func TestBundleKeysIgnoredWithoutIndex(t *testing.T) {
	tool := NewActivateToolsTool(NewActiveTools(), []providers.ToolDef{{Name: "get_flow", Description: "d"}}, nil)
	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{GroupPrefix + CategoryAutomation},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Unknown bundle(s)") || !strings.Contains(out, "No bundles are available here") {
		t.Fatalf("expected an unknown-bundle report:\n%s", out)
	}
}

// TestDeactivateClosesBundle: deactivate_tools accepts a bundle key and closes it.
func TestDeactivateClosesBundle(t *testing.T) {
	active := NewActiveTools()
	tool := bundleFixture(t, active)
	if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{GroupPrefix + CategoryAutomation, "get_flow"},
	})); err != nil {
		t.Fatal(err)
	}
	deact := NewDeactivateToolsTool(active)
	out, err := deact.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{GroupPrefix + CategoryAutomation, "get_flow"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Deactivated: get_flow") || !strings.Contains(out, "Closed bundle(s): group:automation") {
		t.Fatalf("unexpected deactivate result:\n%s", out)
	}
	if active.HasBundle(GroupPrefix + CategoryAutomation) {
		t.Fatal("bundle must be closed")
	}
}

// TestToolSearchTagsBundles: search results carry the match's bundle key so the
// bundle vocabulary is discoverable without a prompt-block change.
func TestToolSearchTagsBundles(t *testing.T) {
	search := NewToolSearchTool([]providers.ToolDef{
		{Name: "get_flow", Description: "get a flow"},
		{Name: "srv__alpha", Description: "flow-ish alpha"},
	})
	out, err := search.Call(context.Background(), mustJSON(t, map[string]any{"query": "flow"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[group:automation]") || !strings.Contains(out, "[mcp:srv]") {
		t.Fatalf("bundle tags missing:\n%s", out)
	}
}
