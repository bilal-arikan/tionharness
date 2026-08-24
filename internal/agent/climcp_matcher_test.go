package agent

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// bBash / bPS are the bridged shell tool names cliMatcherRegex auto-adds.
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
		if got := cliMatcherRegex(c.in); got != c.want {
			t.Errorf("cliMatcherRegex(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCliMatcherRegexFires is the behavioral guard: the converted regex must
// full-match BOTH the plain and the bridged shell tool names (the verbatim comma
// list matched neither), and must NOT over-match a superset name.
func TestCliMatcherRegexFires(t *testing.T) {
	// A plain "Bash,PowerShell" matcher — the common one-click template shape —
	// must now fire on the bridged tools the CLI actually invokes.
	re := regexp.MustCompile(cliMatcherRegex("Bash,PowerShell"))
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

// TestWriteCLISettingsConvertsMatcher is the end-to-end wiring guard: a stored
// comma-glob hook must land in the generated claude-cli settings file as the
// converted+expanded regex (not the verbatim comma list that never fires).
func TestWriteCLISettingsConvertsMatcher(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetCLIHooksEnabled(true)
	ctx := context.Background()
	// A WS15-shaped hook: plain shell matcher, no bridged names — the case this fixes.
	if _, err := rt.db.CreateHook(ctx, db.Hook{
		Event: db.HookPreToolUse, Type: "command", Enabled: true, TimeoutSec: 30,
		Matcher: "Bash,PowerShell",
		Command: "sqz hook claude",
	}); err != nil {
		t.Fatalf("create hook: %v", err)
	}

	path, cleanup, err := rt.writeCLISettings(ctx, nil, "high")
	if err != nil {
		t.Fatalf("writeCLISettings: %v", err)
	}
	defer cleanup()
	if path == "" {
		t.Fatal("expected a settings file path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	s := string(data)
	if want := `^(Bash|` + bBash + `|PowerShell|` + bPS + `)$`; !strings.Contains(s, want) {
		t.Errorf("settings must carry converted+expanded regex %q, got:\n%s", want, s)
	}
	if strings.Contains(s, `"Bash,PowerShell"`) {
		t.Errorf("settings must NOT carry the verbatim comma matcher:\n%s", s)
	}
}
