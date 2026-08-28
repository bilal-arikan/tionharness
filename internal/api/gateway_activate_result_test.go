package api

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
)

// gatewayActivateFixture wires a bare interaction backend with one bound run, the
// same shape TestGatewayDynamicExtendedSurface uses. b.srv stays nil, so no
// tools/list_changed push happens and the "will appear on your next tool list"
// note is part of the pinned text.
func gatewayActivateFixture(t *testing.T) (*interactionBackend, *chatRun, string) {
	t.Helper()
	runs := newChatRuns()
	b := &interactionBackend{runs: runs, tun: agent.NewTunables()}
	run := runs.register("r1", "s1", "ws1", func() {})
	tok := runs.interactionToken("ws1", "s1", "a1")
	runs.bindActive(tok, run)
	return b, run, tok
}

// TestGatewayActivateResultTextIsByteStable pins the EXACT bytes the claude-cli
// gateway returns for activate_tools / deactivate_tools / active_tools. This
// wording is deliberately NOT the native activate_tools wording (internal/tools,
// ActivateToolsTool.Call): the gateway must echo the namespaced callable name and
// the "call by this exact name" contract, and it has no per-tool description or
// always-on set to report. The two are locked separately so neither can drift
// into the other by accident.
func TestGatewayActivateResultTextIsByteStable(t *testing.T) {
	b, run, tok := gatewayActivateFixture(t)

	res, err := b.callActivate(tok, run, json.RawMessage(`{"tools":["notify"]}`), true)
	if err != nil {
		t.Fatal(err)
	}
	want := "activated: " + extendedNSPrefix + "notify\n" +
		"Call each by this exact (namespaced) name.\n" +
		"(note: tools registered; they will appear on your next tool list)"
	if res.Text != want {
		t.Fatalf("activate result drifted:\n got: %q\nwant: %q", res.Text, want)
	}
	if res.IsError {
		t.Fatalf("a successful activate must not be an error result: %q", res.Text)
	}

	// Re-activating an already-active tool adds nothing.
	res, err = b.callActivate(tok, run, json.RawMessage(`{"tools":["notify"]}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if want := "no new tools activated (already active or none valid)"; res.Text != want {
		t.Fatalf("re-activate result drifted:\n got: %q\nwant: %q", res.Text, want)
	}

	// An unknown name is appended to the same line, and marks the call an error.
	res, err = b.callActivate(tok, run, json.RawMessage(`{"tools":["totally_not_a_tool"]}`), true)
	if err != nil {
		t.Fatal(err)
	}
	want = "no new tools activated (already active or none valid)" +
		"; unknown (not in the on-demand catalog): totally_not_a_tool"
	if res.Text != want {
		t.Fatalf("unknown-name result drifted:\n got: %q\nwant: %q", res.Text, want)
	}
	if !res.IsError {
		t.Fatalf("an all-unknown activate must report IsError, got %q", res.Text)
	}

	// active_tools lists the namespaced callable form, one per line.
	got := b.callActiveTools(tok)
	if want := "Activated tools (call by these exact names):\n- " + extendedNSPrefix + "notify"; got.Text != want {
		t.Fatalf("active_tools result drifted:\n got: %q\nwant: %q", got.Text, want)
	}

	// Deactivation, then the empty case.
	res, err = b.callActivate(tok, run, json.RawMessage(`{"tools":["notify"]}`), false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "deactivated: notify"; res.Text != want {
		t.Fatalf("deactivate result drifted:\n got: %q\nwant: %q", res.Text, want)
	}
	res, err = b.callActivate(tok, run, json.RawMessage(`{"tools":["notify"]}`), false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "no tools deactivated (none were active)"; res.Text != want {
		t.Fatalf("empty deactivate result drifted:\n got: %q\nwant: %q", res.Text, want)
	}

	// active_tools back to empty.
	if got := b.callActiveTools(tok); got.Text != "No on-demand tools activated. Use activate_tools to load one from the 'Available Tools' catalog." {
		t.Fatalf("empty active_tools result drifted: %q", got.Text)
	}
}
