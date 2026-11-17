package climcp

import (
	"regexp"
	"testing"
)

func TestCliMatcherRegex(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"  ", ""},
		{"Bash", "^Bash$"},
		{"Bash,PowerShell", "^(Bash|PowerShell)$"},
		{" Bash , PowerShell ", "^(Bash|PowerShell)$"}, // whitespace tolerant
		{"Bash,,PowerShell", "^(Bash|PowerShell)$"},    // empty alt dropped
		{"Bash,Bash", "^Bash$"},                        // duplicates collapse
		{"http_*", "^http_.*$"},                        // glob star
		{"tool_?", "^tool_.$"},                         // glob question mark
		{"a.b+c", `^a\.b\+c$`},                         // regex metachars escaped
	}
	for _, c := range cases {
		if got := MatcherRegex(c.in); got != c.want {
			t.Errorf("MatcherRegex(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCliMatcherRegexFires is the behavioral guard: the converted regex must
// full-match the CLI's own shell tool names (the verbatim comma list matched
// neither), and must NOT over-match a superset name.
func TestCliMatcherRegexFires(t *testing.T) {
	re := regexp.MustCompile(MatcherRegex("Bash,PowerShell"))
	for _, tool := range []string{"Bash", "PowerShell"} {
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
	verbatim := regexp.MustCompile(regexp.QuoteMeta("Bash,PowerShell"))
	if verbatim.MatchString("Bash") {
		t.Errorf("sanity: verbatim comma matcher should not match a single tool name")
	}
}
