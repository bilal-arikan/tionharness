package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
)

// TestGatewayBundleActivateListsWithoutAdvertising is the CLI-side contract test:
// opening a bundle returns member summaries but registers NOTHING on the extended
// server, so the advertised set (and therefore the schema cost on the wire) does
// not grow.
func TestGatewayBundleActivateListsWithoutAdvertising(t *testing.T) {
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: agent.NewTunables()}
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)

	before := len(b.Tools(tok, "extended"))

	res, err := b.callActivate(tok, run, json.RawMessage(`{"tools":["group:interaction"]}`), true)
	if err != nil {
		t.Fatalf("callActivate err: %v", err)
	}
	if res.IsError {
		t.Fatalf("bundle activate should not error: %q", res.Text)
	}
	if !strings.Contains(res.Text, "group:interaction") || !strings.Contains(res.Text, "summaries only, nothing was activated") {
		t.Fatalf("expected a bundle listing, got:\n%s", res.Text)
	}
	// Members are printed under the namespaced callable form, the only name the CLI
	// can call after a follow-up per-tool activate.
	if !strings.Contains(res.Text, extendedNSPrefix+"notify") {
		t.Fatalf("member must be listed namespaced, got:\n%s", res.Text)
	}
	if after := len(b.Tools(tok, "extended")); after != before {
		t.Fatalf("bundle activate advertised %d extra tool(s) on the extended tier", after-before)
	}
	if res := b.callActiveTools(tok); !strings.Contains(res.Text, "No on-demand tools") {
		t.Fatalf("no tool may be activated by a bundle open, got %q", res.Text)
	}
}

// TestGatewayBundleUnknownKey: a bad key reports the known bundles instead of
// silently activating nothing.
func TestGatewayBundleUnknownKey(t *testing.T) {
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: agent.NewTunables()}
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)

	res, _ := b.callActivate(tok, run, json.RawMessage(`{"tools":["group:automaton"]}`), true)
	if !strings.Contains(res.Text, "unknown bundle: group:automaton") || !strings.Contains(res.Text, "known bundles:") {
		t.Fatalf("expected an unknown-bundle report, got:\n%s", res.Text)
	}
}

// TestGatewayBundleNoopOnFullTierProvider: codex-cli is already shown every
// non-core tool, so a bundle listing buys it nothing (defensive — gatewayMetaTools
// strips activate_tools from its list entirely).
func TestGatewayBundleNoopOnFullTierProvider(t *testing.T) {
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: agent.NewTunables()}
	run := runs.register("r1", "s1", "ws1", func() {})
	run.setProvider("codex-cli")
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)

	res, _ := b.callActivate(tok, run, json.RawMessage(`{"tools":["group:interaction"]}`), true)
	if !strings.Contains(res.Text, "already advertised on this provider") {
		t.Fatalf("expected the full-tier short-circuit, got:\n%s", res.Text)
	}
}

// TestGatewayMixedNamesAndBundle: a name travelling with a bundle key still
// activates normally, and only that name is advertised.
func TestGatewayMixedNamesAndBundle(t *testing.T) {
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: agent.NewTunables()}
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)

	res, _ := b.callActivate(tok, run, json.RawMessage(`{"tools":["notify","group:interaction"]}`), true)
	if !strings.Contains(res.Text, "activated: ") || !strings.Contains(res.Text, "group:interaction (") {
		t.Fatalf("expected both an activation and a listing, got:\n%s", res.Text)
	}
	ext := b.Tools(tok, "extended")
	if !specHasTool(ext, "notify") {
		t.Fatalf("the named tool must be advertised, got %v", specNames(ext))
	}
	if len(ext) != 1 {
		t.Fatalf("only the named tool may be advertised, got %v", specNames(ext))
	}
}
