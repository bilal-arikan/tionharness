package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// bundleListRun builds a gateway run whose activatable universe is exactly the
// two bridged defs below, so a bundle listing over it is deterministic.
func bundleListRun(t *testing.T) (*interactionBackend, *chatRun) {
	t.Helper()
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: agent.NewTunables()}
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)
	run.setBridge(
		[]providers.ToolDef{
			{Name: "zzz_alpha", Description: "alpha desc"},
			{Name: "zzz_beta", Description: "beta desc"},
		},
		func(_ context.Context, name string, _ json.RawMessage) (string, error) { return "did:" + name, nil },
	)
	run.setTierVis(func(string) string { return tools.VisibilityNameOnly })
	run.setToolAllowed(func(n string) bool { return n == "zzz_alpha" || n == "zzz_beta" })
	return b, run
}

// TestGatewayBundleListingGolden is a CHARACTERIZATION test: it pins the exact
// bytes listBundles produced before its rendering moved into tools.RenderBundleList.
// Note the gateway prints members under their NAMESPACED callable name — the only
// name the CLI can activate — unlike the native path, which prints bare names.
func TestGatewayBundleListingGolden(t *testing.T) {
	b, run := bundleListRun(t)
	key := tools.BundleOf("zzz_alpha")
	if key == "" || key != tools.BundleOf("zzz_beta") {
		t.Fatalf("test fixture assumes both defs share one bundle, got %q / %q", key, tools.BundleOf("zzz_beta"))
	}
	got := b.listBundles(run, []string{key})
	want := key + " (2 tools) — summaries only, nothing was activated. Load one with activate_tools(\"<name>\").\n" +
		"- " + extendedNSPrefix + "zzz_alpha — alpha desc\n" +
		"- " + extendedNSPrefix + "zzz_beta — beta desc"
	if got != want {
		t.Fatalf("gateway bundle listing drifted.\n got: %q\nwant: %q", got, want)
	}
}

// TestGatewayBundleListingUnknownGolden pins the unknown-key branch, which stays
// on the gateway side: the native path reports unknown keys from its own caller.
func TestGatewayBundleListingUnknownGolden(t *testing.T) {
	b, run := bundleListRun(t)
	got := b.listBundles(run, []string{"group:nope"})
	if !strings.HasPrefix(got, "unknown bundle: group:nope; known bundles: ") {
		t.Fatalf("unknown-bundle wording drifted: %q", got)
	}
}
