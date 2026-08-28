package tools

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// activateResultFixture builds an activate_tools over a small catalog with one
// always-on (eager) tool, so every branch of the result text — activated rows,
// already-active, always-on and unknown — can be exercised in one call.
func activateResultFixture(active *ActiveTools) ActivateToolsTool {
	catalog := []providers.ToolDef{
		{Name: "get_flow", Description: "get a flow"},
		{Name: "list_flows", Description: "list flows"},
	}
	eager := map[string]bool{"read_file": true}
	return NewActivateToolsToolBundled(active, catalog, eager, nil)
}

// TestActivateResultTextIsByteStable pins the EXACT bytes activate_tools returns.
// The gateway (internal/api, callActivateNames) answers the same tool with a
// completely different wording on purpose — namespaced names on one line plus a
// call contract sentence — so there is nothing to share here and each side must be
// locked independently against drift.
func TestActivateResultTextIsByteStable(t *testing.T) {
	tool := activateResultFixture(NewActiveTools())

	got, err := tool.Call(context.Background(), mustJSON(t, map[string]any{
		"names": []string{"get_flow", "list_flows", "read_file", "bogus"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	const want = "Activated 2 tool(s). Their schemas arrive on your NEXT step — do not call them in this same response:\n" +
		"- get_flow — get a flow\n" +
		"- list_flows — list flows\n" +
		"Already available (always-on, no activation needed): read_file\n" +
		"Unknown (skipped): bogus"
	if got != want {
		t.Fatalf("activate result drifted:\n got: %q\nwant: %q", got, want)
	}
}

// TestActivateResultAlreadyActive pins the re-activation wording.
func TestActivateResultAlreadyActive(t *testing.T) {
	active := NewActiveTools()
	tool := activateResultFixture(active)
	if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"names": []string{"get_flow"}})); err != nil {
		t.Fatal(err)
	}

	got, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"names": []string{"get_flow"}}))
	if err != nil {
		t.Fatal(err)
	}
	const want = "Already active: get_flow"
	if got != want {
		t.Fatalf("re-activation result drifted:\n got: %q\nwant: %q", got, want)
	}
}

// TestActivateResultNothingToActivate pins the two no-activation branches: an
// always-on-only request and an unknown-only request never reach the active set.
func TestActivateResultNothingToActivate(t *testing.T) {
	tool := activateResultFixture(NewActiveTools())

	cases := []struct {
		name  string
		names []string
		want  string
	}{
		{"empty", []string{}, "No tool names given."},
		{
			"always-on only",
			[]string{"read_file"},
			"Nothing to activate: read_file already available (always-on) — just call it directly.",
		},
		{
			"unknown only",
			[]string{"bogus"},
			"Unknown names: bogus. Use the exact names from the \"Available Tools (load on demand)\" list (or tool_search).",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"names": tc.names}))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("result drifted:\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestDeactivateResultTextIsByteStable pins the deactivate_tools result text. The
// gateway answers "deactivated: <names>" / "no tools deactivated (none were
// active)" instead — again a separate wording, locked separately.
func TestDeactivateResultTextIsByteStable(t *testing.T) {
	active := NewActiveTools()
	tool := activateResultFixture(active)
	if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"names": []string{"get_flow"}})); err != nil {
		t.Fatal(err)
	}
	deact := NewDeactivateToolsTool(active)

	got, err := deact.Call(context.Background(), mustJSON(t, map[string]any{"names": []string{"get_flow"}}))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Deactivated: get_flow"; got != want {
		t.Fatalf("deactivate result drifted:\n got: %q\nwant: %q", got, want)
	}

	got, err = deact.Call(context.Background(), mustJSON(t, map[string]any{"names": []string{"get_flow"}}))
	if err != nil {
		t.Fatal(err)
	}
	if want := "No active tools or open bundles matched; nothing deactivated."; got != want {
		t.Fatalf("empty deactivate result drifted:\n got: %q\nwant: %q", got, want)
	}
}
