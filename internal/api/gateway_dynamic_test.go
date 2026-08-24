package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/interaction"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
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
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
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
// namespaced name the catalog shows (mcp__tionharness_extended__notify), not just the
// bare form — the model may echo either.
func TestGatewayActivateAcceptsNamespacedName(t *testing.T) {
	tun := agent.NewTunables()
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: tun}
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)

	res, _ := b.callActivate(tok, run, json.RawMessage(`{"tools":["mcp__tionharness_extended__notify"]}`), true)
	if res.IsError {
		t.Fatalf("namespaced activate should succeed, got %q", res.Text)
	}
	if ext := b.Tools(tok, "extended"); !specHasTool(ext, "notify") {
		t.Fatalf("notify must be active after namespaced activate, got %v", specNames(ext))
	}
	// The result must name the NAMESPACED callable form, not the bare name: echoing
	// "activated: notify" made the model call bare `notify` and hit "No such tool
	// available: notify" before retrying (SES125 / list_agents). Report the exact
	// callable name so there is no mis-address round-trip.
	if want := extendedNSPrefix + "notify"; !strings.Contains(res.Text, want) {
		t.Fatalf("activate result must name the namespaced callable %q, got %q", want, res.Text)
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
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
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

// TestFullTierBypassesGatewayGate locks the codex-cli variant of the gateway
// request (tier suffixed "-full", see fullTierQueryParam in package interaction /
// interactionServers in internal/agent/codexmcp.go): since codex-cli never
// re-fetches tools/list on a tools/list_changed push, the lazy activate/deactivate
// gate can never open for it, so a "-full" request must (a) advertise the COMPLETE
// extended tier unconditionally — no activation needed — and (b) drop the gateway
// meta-tools (activate_tools/deactivate_tools/active_tools/tool_search) from BOTH
// the core and extended halves, since offering a mechanism codex can never
// complete only invites a call→no-op→retry loop. The plain "core"/"extended"
// request (claude-cli) must keep its EXACT existing behavior — gated extended
// tier, meta-tools present on core, empty extended until activated.
func TestFullTierBypassesGatewayGate(t *testing.T) {
	tun := agent.NewTunables()
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: tun}
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)

	// Bridge a hidden-classified tool too, so the full-tier assertion also covers
	// the hidden→extended fold (§7-15) under bypass.
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

	// --- claude-cli path (plain tier): behavior must be completely unchanged. ---
	if ext := b.Tools(tok, "extended"); len(ext) != 0 {
		t.Fatalf("plain extended tier must still start EMPTY (claude-cli path unchanged), got %v", specNames(ext))
	}
	core := b.Tools(tok, "core")
	for _, meta := range []string{"activate_tools", "deactivate_tools", "active_tools", "tool_search"} {
		if !specHasTool(core, meta) {
			t.Fatalf("plain core tier must still advertise %s (claude-cli path unchanged), got %v", meta, specNames(core))
		}
	}
	if specHasTool(b.Tools(tok, "extended"), "secret_ops") {
		t.Fatal("plain extended tier must not advertise the hidden tool before activation")
	}

	// --- codex-cli path ("-full" tier): no gate, no meta-tools. ---
	fullExt := b.Tools(tok, "extended-full")
	if !specHasTool(fullExt, "notify") {
		t.Fatalf("full extended tier must advertise a normal extended candidate (notify) unconditionally, got %v", specNames(fullExt))
	}
	if !specHasTool(fullExt, "secret_ops") {
		t.Fatalf("full extended tier must also advertise the hidden tool unconditionally, got %v", specNames(fullExt))
	}
	for _, meta := range []string{"activate_tools", "deactivate_tools", "active_tools", "tool_search"} {
		if specHasTool(fullExt, meta) {
			t.Fatalf("full extended tier must NOT advertise the gateway meta-tool %s (codex can never complete the activation loop), got %v", meta, specNames(fullExt))
		}
	}

	fullCore := b.Tools(tok, "core-full")
	if !specHasTool(fullCore, "ask_user") {
		t.Fatalf("full core tier must still advertise ordinary core tools, got %v", specNames(fullCore))
	}
	for _, meta := range []string{"activate_tools", "deactivate_tools", "active_tools", "tool_search"} {
		if specHasTool(fullCore, meta) {
			t.Fatalf("full core tier must NOT advertise the gateway meta-tool %s, got %v", meta, specNames(fullCore))
		}
	}

	// The plain-tier request must STILL be gated after a full-tier call was made —
	// the "-full" flag must not leak state across requests/tiers.
	if ext := b.Tools(tok, "extended"); len(ext) != 0 {
		t.Fatalf("a full-tier request must not mutate the plain tier's gate, got %v", specNames(ext))
	}
}

// TestInteractionToolsHonorDisabled locks the claude-cli bridge to the agent's
// effective tool filter: a tool the filter rejects (workspace DisabledTools or the
// agent denylist) must NOT be advertised in tools/list, nor be activatable — matching
// the native ToolCatalog. Regression for the gap where a workspace that disabled
// PowerShell (to force Bash so the sqz/rtk optimizer, which only rewrites Bash,
// applies) still saw the bridged PowerShell tool and could call it. Uses core/extended
// built-ins (create_artifact / notify) so the assertion does not depend on which host
// shells ShellToolNames() resolves.
func TestInteractionToolsHonorDisabled(t *testing.T) {
	tun := agent.NewTunables()
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: tun}
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)

	// Baseline (no filter installed): create_artifact is on the core tier and notify
	// is an activatable extended candidate.
	if !specHasTool(b.Tools(tok, "core"), "create_artifact") {
		t.Fatalf("baseline: create_artifact must be on the core tier, got %v", specNames(b.Tools(tok, "core")))
	}
	if !b.extendedCandidates(run)["notify"] {
		t.Fatal("baseline: notify must be an activatable candidate")
	}

	// Reject both via the effective tool-allow predicate the run now carries.
	run.setToolAllowed(func(name string) bool { return name != "create_artifact" && name != "notify" })

	if specHasTool(b.Tools(tok, "core"), "create_artifact") {
		t.Fatalf("create_artifact must be dropped once the filter rejects it, got %v", specNames(b.Tools(tok, "core")))
	}
	if !specHasTool(b.Tools(tok, "core"), "use_skill") {
		t.Fatalf("an unrelated core tool (use_skill) must stay advertised, got %v", specNames(b.Tools(tok, "core")))
	}
	if b.extendedCandidates(run)["notify"] {
		t.Fatal("a filter-rejected tool must not be activatable")
	}
}
