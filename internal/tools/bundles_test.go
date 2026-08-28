package tools

import (
	"reflect"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/mcp"
)

func TestBundleOf(t *testing.T) {
	cases := map[string]string{
		"create_flow":       GroupPrefix + CategoryAutomation,
		"Read":              GroupPrefix + CategoryFiles,
		"totally_unlisted":  GroupPrefix + CategoryOther,
		"playwright__click": MCPBundlePrefix + "playwright",
	}
	for name, want := range cases {
		if got := BundleOf(name); got != want {
			t.Errorf("BundleOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestSplitAndValidBundleKey(t *testing.T) {
	kind, value, ok := SplitBundleKey("mcp:playwright")
	if kind != "mcp" || value != "playwright" || !ok {
		t.Fatalf("SplitBundleKey(mcp:playwright) = %q,%q,%v", kind, value, ok)
	}
	kind, value, ok = SplitBundleKey("group:automation")
	if kind != "group" || value != "automation" || !ok {
		t.Fatalf("SplitBundleKey(group:automation) = %q,%q,%v", kind, value, ok)
	}
	if _, _, ok := SplitBundleKey("notify"); ok {
		t.Error("a plain tool name must not parse as a bundle key")
	}

	valid := []string{"group:automation", "group:other", "mcp:playwright", MCPBundleWildcard}
	for _, k := range valid {
		if !ValidBundleKey(k) {
			t.Errorf("ValidBundleKey(%q) = false, want true", k)
		}
	}
	invalid := []string{"group:automaton", "group:", "mcp:", "notify", ""}
	for _, k := range invalid {
		if ValidBundleKey(k) {
			t.Errorf("ValidBundleKey(%q) = true, want false", k)
		}
	}
}

func TestMatchesBundle(t *testing.T) {
	if !MatchesBundle("create_flow", "group:automation") {
		t.Error("built-in must match its category bundle")
	}
	if MatchesBundle("srv__tool", "group:automation") {
		t.Error("an MCP tool must never match a group bundle")
	}
	if !MatchesBundle("srv__tool", "mcp:srv") {
		t.Error("MCP tool must match its server bundle")
	}
	if MatchesBundle("srv__tool", "mcp:other") {
		t.Error("MCP tool must not match another server's bundle")
	}
	if !MatchesBundle("srv__tool", MCPBundleWildcard) {
		t.Error("the mcp wildcard must match every namespaced tool")
	}
	if MatchesBundle("create_flow", MCPBundleWildcard) {
		t.Error("the mcp wildcard must not match a built-in")
	}
	// MatchesGroup keeps its own (built-in only) semantics.
	if MatchesGroup("srv__tool", "mcp:srv") {
		t.Error("MatchesGroup must stay group-only")
	}
}

func TestBundleIndexGroupsBuiltinsAndMCP(t *testing.T) {
	reg := NewRegistry()
	reg.Add(NewTodoWriteTool())
	reg.AttachMCP([]mcp.CatalogEntry{
		{Server: "srv", NamespacedName: "srv__b"},
		{Server: "srv", NamespacedName: "srv__a"},
	}, nil, nil)

	idx := reg.BundleIndex(nil)
	if got := idx[MCPBundlePrefix+"srv"]; !reflect.DeepEqual(got, []string{"srv__a", "srv__b"}) {
		t.Errorf("mcp bundle members = %v, want sorted [srv__a srv__b]", got)
	}
	if got := idx[GroupPrefix+CategoryAutomation]; !reflect.DeepEqual(got, []string{"todo_write"}) {
		t.Errorf("automation bundle members = %v, want [todo_write]", got)
	}

	filtered := reg.BundleIndex(func(name string) bool { return name != "srv__a" })
	if got := filtered[MCPBundlePrefix+"srv"]; !reflect.DeepEqual(got, []string{"srv__b"}) {
		t.Errorf("filtered mcp bundle members = %v, want [srv__b]", got)
	}
}
