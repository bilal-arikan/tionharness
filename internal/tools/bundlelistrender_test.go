package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestNativeBundleListingGolden is a CHARACTERIZATION test: it pins the exact
// bytes the native activate_tools bundle listing produced before the renderer was
// shared with the gateway path, so the refactor cannot silently reword a line.
func TestNativeBundleListingGolden(t *testing.T) {
	catalog := []providers.ToolDef{
		{Name: "srv__alpha", Description: "alpha desc"},
		{Name: "srv__beta", Description: "beta desc"},
	}
	tool := NewActivateToolsToolBundled(NewActiveTools(), catalog, nil, map[string][]string{
		MCPBundlePrefix + "srv": {"srv__alpha", "srv__beta"},
	})
	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{MCPBundlePrefix + "srv"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	const want = "Opened mcp:srv (2 tools) — summaries only, no schema loaded. Load one with activate_tools(\"<name>\").\n" +
		"- srv__alpha — alpha desc\n" +
		"- srv__beta — beta desc"
	if out != want {
		t.Fatalf("native bundle listing drifted.\n got: %q\nwant: %q", out, want)
	}
}

// TestNativeBundleListingOverflowGolden pins the native truncation notice.
func TestNativeBundleListingOverflowGolden(t *testing.T) {
	total := BundleListLimit + 2
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
	head := fmt.Sprintf("Opened mcp:srv (%d tools) — summaries only, no schema loaded. Load one with activate_tools(\"<name>\").\n", total)
	if !strings.HasPrefix(out, head) {
		t.Fatalf("header drifted:\n%s", out)
	}
	if want := "…and 2 more not shown; narrow with tool_search(\"<keyword>\")."; !strings.HasSuffix(out, want) {
		t.Fatalf("overflow notice drifted, want suffix %q in:\n%s", want, out)
	}
	if got := strings.Count(out, "\n- "); got != BundleListLimit {
		t.Fatalf("listed %d members, want %d", got, BundleListLimit)
	}
}

// TestRenderBundleListRequiresNameOf: an absent name resolver is a programming
// error, not a reason to silently print a bare (possibly uncallable) name.
func TestRenderBundleListRequiresNameOf(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RenderBundleList must panic without a NameOf resolver")
		}
	}()
	RenderBundleList([]BundleListing{{Key: "mcp:srv"}}, BundleListOpts{
		HeaderFormat:   "%s (%d)\n",
		OverflowFormat: "…%d\n",
	})
}

// TestRenderBundleListSharedByBothPaths: the helper is the single point of change
// for both callers. Rendering the same bundle with each path's options differs
// ONLY in the parameterised parts (header wording, printed name form, overflow
// wording) — the row shape itself comes from one place, so a change there lands
// on both paths at once.
func TestRenderBundleListSharedByBothPaths(t *testing.T) {
	listing := []BundleListing{{
		Key:     "mcp:srv",
		Members: []BundleListRow{{Name: "srv__alpha", Desc: "alpha desc"}},
	}}
	native := RenderBundleList(listing, BundleListOpts{
		HeaderFormat:   nativeBundleHeaderFormat,
		OverflowFormat: nativeBundleOverflowFormat,
		Max:            BundleListLimit,
		NameOf:         func(n string) string { return n },
	})
	gateway := RenderBundleList(listing, BundleListOpts{
		HeaderFormat:   "%s (%d tools) — summaries only, nothing was activated. Load one with activate_tools(\"<name>\").\n",
		OverflowFormat: "…and %d more not shown; narrow with tool_search.\n",
		Max:            BundleListLimit,
		NameOf:         func(n string) string { return "ns__" + n },
	})
	const row = " — alpha desc\n"
	if !strings.HasSuffix(native, "- srv__alpha"+row) {
		t.Fatalf("native row shape drifted:\n%s", native)
	}
	if !strings.HasSuffix(gateway, "- ns__srv__alpha"+row) {
		t.Fatalf("gateway row shape drifted:\n%s", gateway)
	}
}
