package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestGatewayToolSearchRenderGolden pins the GATEWAY tool_search result text byte for
// byte. The goldens were captured from the pre-refactor renderer, so they prove the
// move to the shared tools.RenderToolSearch helper changed no output on this path
// either: descriptions capped at 100 bytes, MCP-namespaced names left untagged, and
// an overflow notice that names only the bundles it could resolve.
func TestGatewayToolSearchRenderGolden(t *testing.T) {
	tun := agent.NewTunables()
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: tun}
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)

	defs := []providers.ToolDef{
		{Name: "zeta_probe", Description: "a very long description that keeps going and going past one hundred characters so truncation behaviour becomes observable in the rendered row"},
		{Name: "alpha_probe", Description: "short probe desc"},
		{Name: "mcp__srv__probe", Description: "mcp probe desc"},
	}
	for i := 0; i < 35; i++ {
		defs = append(defs, providers.ToolDef{Name: fmt.Sprintf("bulk_probe_%02d", i), Description: "bulk probe desc"})
	}
	run.setBridge(defs, func(_ context.Context, name string, _ json.RawMessage) (string, error) { return "did:" + name, nil })
	run.setTierVis(func(string) string { return tools.VisibilityNameOnly })

	for _, tc := range []struct{ query, golden string }{
		{"probe", "toolsearch_gateway_overflow.txt"},
		{"zeta", "toolsearch_gateway_longdesc.txt"},
	} {
		got := b.callToolSearch(run, json.RawMessage(`{"query":"`+tc.query+`"}`)).Text
		raw, err := os.ReadFile(filepath.Join("testdata", tc.golden))
		if err != nil {
			t.Fatalf("read golden %s: %v", tc.golden, err)
		}
		if want := strings.ReplaceAll(string(raw), "\r\n", "\n"); got != want {
			t.Fatalf("query %q output drifted from %s:\n--- got ---\n%s\n--- want ---\n%s", tc.query, tc.golden, got, want)
		}
	}
}
