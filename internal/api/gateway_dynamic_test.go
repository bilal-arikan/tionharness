package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/interaction"
)

func specNames(specs []interaction.ToolSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Name
	}
	return out
}

// TestGatewayDynamicExtendedSurface locks Doc 52 Faz 1-b: with GatewayDynamicExtended
// on, the extended tier starts EMPTY, the core tier advertises activate_tools/
// deactivate_tools, and activating a tool makes it appear on the extended tier
// (deactivating removes it). Unknown names are reported, not silently registered.
func TestGatewayDynamicExtendedSurface(t *testing.T) {
	tun := agent.NewTunables()
	tun.SetGatewayDynamicExtended(true)

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

// TestGatewayDynamicExtendedOffIsFullSurface verifies the default (flag off): the
// extended tier advertises its full set and the meta-tools are NOT present.
func TestGatewayDynamicExtendedOffIsFullSurface(t *testing.T) {
	tun := agent.NewTunables() // GatewayDynamicExtended defaults off
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: tun}
	run := runs.register("r1", "s1", func() {})
	tok := runs.interactionToken("s1", "a1")
	runs.bindActive(tok, run)

	ext := b.Tools(tok, "extended")
	if len(ext) == 0 {
		t.Fatal("with the flag off the extended tier must advertise its full set, got empty")
	}
	core := b.Tools(tok, "core")
	if specHasTool(core, "activate_tools") || specHasTool(core, "deactivate_tools") {
		t.Fatalf("meta-tools must not be advertised when the flag is off, got %v", specNames(core))
	}
}

// TestGatewayActivateAcceptsNamespacedName verifies activate_tools accepts the
// namespaced name the catalog shows (mcp__tionswarm_extended__notify), not just the
// bare form — the model may echo either.
func TestGatewayActivateAcceptsNamespacedName(t *testing.T) {
	tun := agent.NewTunables()
	tun.SetGatewayDynamicExtended(true)
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
