package agent

import (
	"context"
	"encoding/json"
	"strings"
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

// TestBridgeToolsWebToolsAreCodexOnly is the callability half of the provider-aware
// web-tool split: catalogDisplayName only decides what the PROMPT names, but a codex
// agent can call WebFetch/WebSearch over the Interaction MCP only if BridgeTools
// actually advertises them (the /core and /extended tool lists are built from these
// defs, and the bridge dispatcher is the same closure returned here). claude-cli must
// stay unchanged — it has its own natives.
func TestBridgeToolsWebToolsAreCodexOnly(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())

	codex, _ := bridgedNames(t, rt, db.Agent{ID: "a-codex", Provider: providerKindCodexCLI})
	for _, name := range []string{"WebFetch", "WebSearch"} {
		if !codex[name] {
			t.Errorf("codex-cli bridge must advertise %s (codex has no native WebFetch and web_search is off by default)", name)
		}
	}

	claude, _ := bridgedNames(t, rt, db.Agent{ID: "a-claude", Provider: "claude-cli"})
	for _, name := range []string{"WebFetch", "WebSearch"} {
		if claude[name] {
			t.Errorf("claude-cli bridge must NOT advertise %s — the CLI has its own native", name)
		}
	}
}

// TestBridgeToolsWebToolsDispatch proves the advertised names are reachable through
// the bridge dispatcher itself (the closure the Interaction backend calls on an
// /extended or /core tools/call). A reachable tool answers about its own arguments;
// an unbridged one comes back as "unknown tool".
func TestBridgeToolsWebToolsDispatch(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	_, call := bridgedNames(t, rt, db.Agent{ID: "a-codex", Provider: providerKindCodexCLI})

	// Empty args: the tool's own validation must answer, which only happens if the
	// dispatcher resolved the name against the registry.
	for _, name := range []string{"WebFetch", "WebSearch"} {
		out, err := call(context.Background(), name, json.RawMessage(`{}`))
		if err == nil && strings.TrimSpace(out) == "" {
			t.Errorf("%s dispatch returned nothing", name)
		}
		msg := out
		if err != nil {
			msg = err.Error()
		}
		if strings.Contains(strings.ToLower(msg), "unknown tool") {
			t.Errorf("%s must be dispatchable through the CLI bridge, got %q", name, msg)
		}
	}
}

// TestBridgeToolsWebToolsRespectAgentDenylist keeps the bridge honest about the
// per-agent filter: a codex agent that has WebSearch denied must not see it
// advertised, exactly like every other bridged tool.
func TestBridgeToolsWebToolsRespectAgentDenylist(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	agent := db.Agent{ID: "a-codex", Provider: providerKindCodexCLI, BlockedTools: `["WebSearch"]`}
	names, _ := bridgedNames(t, rt, agent)
	if names["WebSearch"] {
		t.Error("a denied WebSearch must not be advertised to the codex bridge")
	}
	if !names["WebFetch"] {
		t.Error("denying WebSearch must not take WebFetch with it")
	}
}
