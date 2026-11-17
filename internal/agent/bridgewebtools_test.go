package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// bridgedNames collects the names BridgeTools advertises for an agent.
func bridgedNames(t *testing.T, rt *Runtime, agent db.Agent) (map[string]bool, func(ctx context.Context, name string, args json.RawMessage) (string, error)) {
	t.Helper()
	defs, call := rt.BridgeTools(context.Background(), agent)
	out := make(map[string]bool, len(defs))
	for _, d := range defs {
		out[d.Name] = true
	}
	return out, call
}

// TestBridgeToolsNoWebTools: no CLI gets TionHarness's WebFetch/WebSearch over
// the bridge. claude-cli and opencode-cli ship their own, and codex-cli's native
// web_search ("live") both searches and opens pages; a bridged copy would only
// put a second web path on the menu.
func TestBridgeToolsNoWebTools(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	for _, ag := range []db.Agent{
		{ID: "a-codex", Provider: providerKindCodexCLI},
		{ID: "a-claude", Provider: "claude-cli"},
		{ID: "a-opencode", Provider: providerKindOpenCodeCLI},
	} {
		names, _ := bridgedNames(t, rt, ag)
		for _, name := range []string{"WebFetch", "WebSearch"} {
			if names[name] {
				t.Errorf("%s bridge must NOT advertise %s", ag.Provider, name)
			}
		}
	}
}

// TestBridgeToolsNoFSTools: no CLI gets TionHarness's file tools over the
// bridge. codex reads and edits through its own shell and apply_patch, and
// claude-cli has its own natives; either way a bridged copy would only duplicate.
func TestBridgeToolsNoFSTools(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	for _, ag := range []db.Agent{
		{ID: "a-codex", Provider: providerKindCodexCLI},
		{ID: "a-claude", Provider: "claude-cli"},
	} {
		names, _ := bridgedNames(t, rt, ag)
		for _, name := range []string{"Read", "LS", "Glob", "Grep", "Write", "Edit", "Bash", "PowerShell"} {
			if names[name] {
				t.Errorf("%s bridge must NOT advertise %s", ag.Provider, name)
			}
		}
	}
}
