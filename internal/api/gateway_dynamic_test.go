package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/interaction"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

func specNames(specs []interaction.ToolSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Name
	}
	return out
}

// TestGatewayDynamicExtendedSurface locks Doc 52 Faz 1-b: the claude-cli extended tier
// starts EMPTY, the core tier advertises activate_tools/deactivate_tools/active_tools, and
// activating a tool makes it appear on the extended tier (deactivating removes it).
// Unknown names are reported, not silently registered.
func TestGatewayDynamicExtendedSurface(t *testing.T) {
	tun := agent.NewTunables()

	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: tun}
	run := runs.register("r1", "s1", func() {})
	tok := runs.interactionToken("s1", "a1")
	runs.bindActive(tok, run)

	// Extended starts empty (nothing activated yet).
	if ext := b.Tools(tok, "extended"); len(ext) != 0 {
		t.Fatalf("extended tier must start empty, got %v", specNames(ext))
	}

	// Core advertises the full gateway meta set: activate / deactivate / active.
	core := b.Tools(tok, "core")
	for _, meta := range []string{"activate_tools", "deactivate_tools", "active_tools"} {
		if !specHasTool(core, meta) {
			t.Fatalf("core tier must advertise %s, got %v", meta, specNames(core))
		}
	}

	// active_tools reports nothing before any activation.
	if res := b.callActiveTools(tok); !strings.Contains(res.Text, "No on-demand tools") {
		t.Fatalf("active_tools should report empty before activation, got %q", res.Text)
	}

	// Activate a real extended candidate (notify is name-only → extended).
	res, err := b.callActivate(tok, run, json.RawMessage(`{"tools":["notify"]}`), true)
	if err != nil {
		t.Fatalf("callActivate err: %v", err)
	}
	if res.IsError {
		t.Fatalf("activate should succeed for a valid tool, got error: %q", res.Text)
	}
	if ext := b.Tools(tok, "extended"); !specHasTool(ext, "notify") {
		t.Fatalf("notify must appear on the extended tier after activation, got %v", specNames(ext))
	}

	// active_tools now lists the activated tool.
	if res := b.callActiveTools(tok); !strings.Contains(res.Text, "notify") {
		t.Fatalf("active_tools should list notify after activation, got %q", res.Text)
	}

	// Unknown names are reported (not silently dropped).
	res, _ = b.callActivate(tok, run, json.RawMessage(`{"tools":["totally_not_a_tool"]}`), true)
	if !strings.Contains(res.Text, "unknown") {
		t.Fatalf("expected 'unknown' feedback for a bogus name, got %q", res.Text)
	}

	// Deactivate removes it again.
	if _, err := b.callActivate(tok, run, json.RawMessage(`{"tools":["notify"]}`), false); err != nil {
		t.Fatalf("deactivate err: %v", err)
	}
	if ext := b.Tools(tok, "extended"); specHasTool(ext, "notify") {
		t.Fatalf("notify must be gone from the extended tier after deactivation, got %v", specNames(ext))
	}
}

// TestGatewayActivateAcceptsNamespacedName verifies activate_tools accepts the
// namespaced name the catalog shows (mcp__tionswarm_extended__notify), not just the
// bare form — the model may echo either.
func TestGatewayActivateAcceptsNamespacedName(t *testing.T) {
	tun := agent.NewTunables()
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: tun}
	run := runs.register("r1", "s1", func() {})
	tok := runs.interactionToken("s1", "a1")
	runs.bindActive(tok, run)

	res, _ := b.callActivate(tok, run, json.RawMessage(`{"tools":["mcp__tionswarm_extended__notify"]}`), true)
	if res.IsError {
		t.Fatalf("namespaced activate should succeed, got %q", res.Text)
	}
	if ext := b.Tools(tok, "extended"); !specHasTool(ext, "notify") {
		t.Fatalf("notify must be active after namespaced activate, got %v", specNames(ext))
	}
}

// TestGatewayHiddenActivatableAndToolSearch locks Doc 52 §7-15 / follow-up: a HIDDEN-tier
// tool is not advertised up front (zero token cost), is discoverable via tool_search
// (even though it is absent from the catalog), and becomes activatable + advertised on
// the extended tier once turned on — the CLI analogue of native hidden tools.
func TestGatewayHiddenActivatableAndToolSearch(t *testing.T) {
	tun := agent.NewTunables()
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: tun}
	run := runs.register("r1", "s1", func() {})
	tok := runs.interactionToken("s1", "a1")
	runs.bindActive(tok, run)

	// Bridge a hidden-classified tool with a dispatcher.
	run.setBridge(
		[]providers.ToolDef{{Name: "secret_ops", Description: "perform a secret hidden maintenance operation"}},
		func(_ context.Context, name string, _ json.RawMessage) (string, error) { return "did:" + name, nil },
	)
	run.setTierVis(func(name string) string {
		if name == "secret_ops" {
			return tools.VisibilityHidden
		}
		return tools.VisibilityNameOnly
	})

	// Not advertised before activation (hidden + not activated).
	if specHasTool(b.Tools(tok, "extended"), "secret_ops") {
		t.Fatal("hidden tool must NOT be advertised before activation")
	}
	// tool_search finds it despite being absent from the catalog.
	if res := b.callToolSearch(run, json.RawMessage(`{"query":"secret maintenance"}`)); !strings.Contains(res.Text, "secret_ops") {
		t.Fatalf("tool_search must surface the hidden tool, got %q", res.Text)
	}
	// Activating it advertises it on the extended tier.
	if res, _ := b.callActivate(tok, run, json.RawMessage(`{"tools":["secret_ops"]}`), true); res.IsError {
		t.Fatalf("activating a hidden tool must succeed, got %q", res.Text)
	}
	if !specHasTool(b.Tools(tok, "extended"), "secret_ops") {
		t.Fatal("hidden tool must be advertised on the extended tier after activation")
	}
}
