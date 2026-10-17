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
