package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// toolSearchProbeCatalog is the fixture both characterization cases search over:
// one over-long description (does this path truncate?), one MCP-namespaced name
// (does this path tag it?), and enough bulk entries to spill past ToolSearchMaxRows
// (what does the overflow notice say?).
func toolSearchProbeCatalog() []providers.ToolDef {
	cat := []providers.ToolDef{
		{Name: "zeta_probe", Description: "a very long description that keeps going and going past one hundred characters so truncation behaviour becomes observable in the rendered row"},
		{Name: "alpha_probe", Description: "short probe desc"},
		{Name: "mcp__srv__probe", Description: "mcp probe desc"},
	}
	for i := 0; i < 35; i++ {
		cat = append(cat, providers.ToolDef{Name: fmt.Sprintf("bulk_probe_%02d", i), Description: "bulk probe desc"})
	}
	return cat
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// TestToolSearchNativeRenderGolden pins the NATIVE tool_search result text byte for
// byte. The goldens were captured from the pre-refactor renderer, so they prove the
// move to the shared RenderToolSearch helper changed no output: full descriptions,
// an unconditional bundle tag (MCP names included) and an overflow notice that always
// carries the "The rest live in: …" clause.
func TestToolSearchNativeRenderGolden(t *testing.T) {
	tool := NewToolSearchTool(toolSearchProbeCatalog())
	for _, tc := range []struct{ query, golden string }{
		{"probe", "toolsearch_native_overflow.txt"},
		{"zeta", "toolsearch_native_longdesc.txt"},
	} {
		got, err := tool.Call(context.Background(), json.RawMessage(`{"query":"`+tc.query+`"}`))
		if err != nil {
			t.Fatalf("query %q: %v", tc.query, err)
		}
		if want := readGolden(t, tc.golden); got != want {
			t.Fatalf("query %q output drifted from %s:\n--- got ---\n%s\n--- want ---\n%s", tc.query, tc.golden, got, want)
		}
	}
}

// TestRenderToolSearchOptionsDriveBothPaths is the single-point-of-change proof: the
// SAME rows rendered with the native options and with the gateway options differ in
// exactly the three ways the options describe (header wording, description cap,
// untagged-overflow clause), and a change to anything else would move both at once.
func TestRenderToolSearchOptionsDriveBothPaths(t *testing.T) {
	rows := []ToolSearchRow{
		{Name: "alpha", Desc: strings.Repeat("x", 120)},
		{Name: "beta", Desc: "short"},
	}
	untagged := func(string) string { return "" }

	native := RenderToolSearch(rows, ToolSearchRenderOpts{
		Header:   "Matching tools (activate with activate_tools):",
		Max:      1,
		BundleOf: func(string) string { return "group:other" },
	})
	gateway := RenderToolSearch(rows, ToolSearchRenderOpts{
		Header:                  "Matching tools (load with activate_tools):",
		Max:                     1,
		DescLimit:               100,
		BundleOf:                untagged,
		OmitEmptyDroppedBundles: true,
	})

	wantNative := "Matching tools (activate with activate_tools):\n" +
		"- alpha — " + strings.Repeat("x", 120) + "  [group:other]" +
		"\n…and 1 more; refine the query. The rest live in: group:other."
	if native != wantNative {
		t.Fatalf("native options:\n got %q\nwant %q", native, wantNative)
	}
	wantGateway := "Matching tools (load with activate_tools):\n" +
		"- alpha — " + strings.Repeat("x", 100) + "…" +
		"\n…and 1 more; refine the query."
	if gateway != wantGateway {
		t.Fatalf("gateway options:\n got %q\nwant %q", gateway, wantGateway)
	}

	// The shared part really is shared: flip one thing in the row shape source and
	// both renderings move. Here the bundle resolver is the shared input — giving the
	// gateway options a resolver makes its rows carry the tag too, in the identical
	// "  [key]" form the native path uses.
	tagged := RenderToolSearch(rows[1:], ToolSearchRenderOpts{
		Header:                  "H:",
		Max:                     ToolSearchMaxRows,
		DescLimit:               100,
		BundleOf:                func(string) string { return "group:tasks" },
		OmitEmptyDroppedBundles: true,
	})
	if want := "H:\n- beta — short  [group:tasks]"; tagged != want {
		t.Fatalf("shared tag form:\n got %q\nwant %q", tagged, want)
	}
}

// TestRenderToolSearchRejectsMissingBundleResolver keeps a nil resolver loud: silently
// rendering every row untagged is exactly the half-ported bug this helper exists to
// prevent.
func TestRenderToolSearchRejectsMissingBundleResolver(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic when BundleOf is nil")
		}
	}()
	RenderToolSearch([]ToolSearchRow{{Name: "a", Desc: "b"}}, ToolSearchRenderOpts{Header: "H:", Max: 30})
}
