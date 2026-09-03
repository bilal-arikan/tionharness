package providers

import (
	"strings"
	"testing"
)

// TestNativeToolArgs pins the `--tools` rendering: absent when unrestricted, one
// comma-joined argument for a list (the CLI's documented form), and a literal ""
// for the empty list — the CLI's spelling for "no built-in tools".
func TestNativeToolArgs(t *testing.T) {
	if got := nativeToolArgs(Request{}); got != nil {
		t.Fatalf("unrestricted request must add no flag, got %v", got)
	}
	got := nativeToolArgs(Request{CLIRestrictNativeTools: true, CLINativeTools: []string{"Read", "Edit", "ToolSearch"}})
	if len(got) != 2 || got[0] != "--tools" || got[1] != "Read,Edit,ToolSearch" {
		t.Fatalf("restricted list = %v, want [--tools Read,Edit,ToolSearch]", got)
	}
	got = nativeToolArgs(Request{CLIRestrictNativeTools: true})
	if len(got) != 2 || got[0] != "--tools" || got[1] != "" {
		t.Fatalf("empty restriction = %v, want [--tools \"\"]", got)
	}
}

// TestNativeToolArgsReachBothLaunchers guards the wiring: the persistent-session
// fingerprint must change with the restriction (a changed menu is a changed
// process), and the one-shot launcher must carry the flag after the MCP flags.
func TestNativeToolArgsReachBothLaunchers(t *testing.T) {
	c := &ClaudeCLI{}
	plain := c.persistentFingerprint(Request{}, "sys")
	restricted := c.persistentFingerprint(Request{CLIRestrictNativeTools: true, CLINativeTools: []string{"Read"}}, "sys")
	if plain == restricted {
		t.Fatal("persistent fingerprint must include the native tool restriction")
	}
	args := append(c.mcpArgs(), nativeToolArgs(Request{CLIRestrictNativeTools: true, CLINativeTools: []string{"Read"}})...)
	if !strings.Contains(strings.Join(args, " "), "--tools Read") {
		t.Fatalf("launcher args = %v, want --tools Read", args)
	}
}

func TestNativeClaudeModel(t *testing.T) {
	cases := map[string]string{
		"haiku":                     "claude-haiku-4-5-20251001",
		"Sonnet":                    "claude-sonnet-5",
		"opus":                      "claude-opus-5",
		"fable":                     "claude-fable-5-1",
		"claude-opus-4-8":           "claude-opus-4-8",
		"":                          "claude-haiku-4-5-20251001",
		"gpt-5.6-sol":               "claude-haiku-4-5-20251001",
		" claude-sonnet-4-6 ":       "claude-sonnet-4-6",
		"claude-haiku-4-5-20251001": "claude-haiku-4-5-20251001",
	}
	for in, want := range cases {
		if got := NativeClaudeModel(in); got != want {
			t.Errorf("NativeClaudeModel(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestFirstAvailableOfKind covers the routing lookup: only an ENABLED instance
// whose kind reports available (anthropic: a key is set) is returned, ids
// sorted so the pick is deterministic.
func TestFirstAvailableOfKind(t *testing.T) {
	r := NewRegistry()
	r.SetInstances([]Instance{
		{ID: "anthropic-b", KindID: "anthropic", Enabled: true, Values: map[string]string{FieldKeyAPIKey: "sk-b"}},
		{ID: "anthropic-a", KindID: "anthropic", Enabled: true, Values: map[string]string{FieldKeyAPIKey: "sk-a"}},
		{ID: "anthropic-off", KindID: "anthropic", Enabled: false, Values: map[string]string{FieldKeyAPIKey: "sk-off"}},
	})
	if got := r.FirstAvailableOfKind("anthropic"); got != "anthropic-a" {
		t.Fatalf("FirstAvailableOfKind = %q, want anthropic-a (sorted, enabled, keyed)", got)
	}

	r.SetInstances([]Instance{
		{ID: "anthropic", KindID: "anthropic", Enabled: true, Values: map[string]string{}},
	})
	if got := r.FirstAvailableOfKind("anthropic"); got != "" {
		t.Fatalf("keyless anthropic instance must not be picked, got %q", got)
	}
	if got := r.FirstAvailableOfKind("no-such-kind"); got != "" {
		t.Fatalf("unknown kind must yield \"\", got %q", got)
	}
}
