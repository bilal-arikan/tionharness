package climcp

import (
	"regexp"
	"testing"
)

// bBash / bPS are the bridged shell tool names MatcherRegex auto-adds.
const (
	bBash = "mcp__tionharness_interaction__Bash"
	bPS   = "mcp__tionharness_interaction__PowerShell"
)

func TestCliMatcherRegex(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"  ", ""},
		// Shell names auto-expand to their bridged (mcp__tionharness_interaction__*) forms.
		{"Bash", "^(Bash|" + bBash + ")$"},
		{"Bash,PowerShell", "^(Bash|" + bBash + "|PowerShell|" + bPS + ")$"},
		{" Bash , PowerShell ", "^(Bash|" + bBash + "|PowerShell|" + bPS + ")$"}, // whitespace tolerant
		{"Bash,,PowerShell", "^(Bash|" + bBash + "|PowerShell|" + bPS + ")$"},    // empty alt dropped
		// The real WS10 sqz matcher — plain + bridged already listed; must dedup.
		{
			"Bash,PowerShell,mcp__tionharness_interaction__PowerShell,mcp__tionharness_interaction__Bash",
			"^(Bash|" + bBash + "|PowerShell|" + bPS + ")$",
		},
		// Non-shell names are not expanded.
		{"http_*", "^http_.*$"}, // glob star
		{"tool_?", "^tool_.$"},  // glob question mark
		{"a.b+c", `^a\.b\+c$`},  // regex metachars escaped
	}
	for _, c := range cases {
		if got := MatcherRegex(c.in); got != c.want {
			t.Errorf("MatcherRegex(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCliMatcherRegexFires is the behavioral guard: the converted regex must
// full-match BOTH the plain and the bridged shell tool names (the verbatim comma
// list matched neither), and must NOT over-match a superset name.
func TestCliMatcherRegexFires(t *testing.T) {
	// A plain "Bash,PowerShell" matcher — the common one-click template shape —
	// must now fire on the bridged tools the CLI actually invokes.
	re := regexp.MustCompile(MatcherRegex("Bash,PowerShell"))
	for _, tool := range []string{"Bash", "PowerShell", bBash, bPS} {
		if !re.MatchString(tool) {
			t.Errorf("plain Bash,PowerShell matcher must fire on %q", tool)
		}
	}
	for _, tool := range []string{"BashOutput", "PowerShellX", "Write", "mcp__tionharness_extended__list_agents"} {
		if re.MatchString(tool) {
			t.Errorf("matcher must NOT match superset/foreign tool %q", tool)
		}
	}

	// The pre-fix bug reproduction: the verbatim comma list is a regex that matches
	// nothing real, proving why the hook was silent in the CLI path.
	verbatim := regexp.MustCompile(regexp.QuoteMeta("Bash,PowerShell,mcp__tionharness_interaction__PowerShell"))
	if verbatim.MatchString(bPS) {
		t.Errorf("sanity: verbatim comma matcher should not match a single tool name")
	}
}
