package tools

import (
	"encoding/json"
	"testing"
)

func TestParsePermRule(t *testing.T) {
	cases := map[string]PermRule{
		"shell":           {Tool: "shell"},
		"shell(git *)":    {Tool: "shell", ArgGlob: "git *"},
		"  Bash( npm * )": {Tool: "Bash", ArgGlob: "npm *"},
		"write_file":      {Tool: "write_file"},
		"broken(":         {Tool: "broken("}, // no trailing ) → whole-tool, left as-is
	}
	for in, want := range cases {
		if got := ParsePermRule(in); got != want {
			t.Errorf("ParsePermRule(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestPermRuleMatch(t *testing.T) {
	whole := PermRule{Tool: "shell"}
	if !whole.Match("shell", "anything") {
		t.Error("whole-tool rule should match any argument")
	}
	if whole.Match("write_file", "") {
		t.Error("rule must not match a different tool")
	}
	git := PermRule{Tool: "shell", ArgGlob: "git *"}
	if !git.Match("shell", "git status -s") {
		t.Error("git * should match 'git status -s'")
	}
	if git.Match("shell", "rm -rf /") {
		t.Error("git * must not match 'rm -rf /'")
	}
	if git.Match("Bash", "git status") {
		t.Error("rule is scoped to the shell tool, not Bash")
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern, s string
		want       bool
	}{
		{"git *", "git status", true},
		{"git *", "git", false},
		{"*", "anything", true},
		{"npm run *", "npm run build", true},
		{"npm run *", "npm install", false},
		{"exact", "exact", true},
		{"exact", "exactly", false},
		{"a*c", "abc", true},
		{"a*c", "ac", true},
		{"a*c", "ab", false},
	}
	for _, c := range cases {
		if got := globMatch(c.pattern, c.s); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}

func TestRepresentativeArg(t *testing.T) {
	cmd := json.RawMessage(`{"command":"git status"}`)
	if got := RepresentativeArg("shell", cmd); got != "git status" {
		t.Errorf("shell command arg = %q, want %q", got, "git status")
	}
	if got := RepresentativeArg("Bash", json.RawMessage(`{"command":" ls -la "}`)); got != "ls -la" {
		t.Errorf("Bash command arg = %q, want trimmed %q", got, "ls -la")
	}
	// Non-exec tools never expose an argument (only whole-tool grants apply).
	if got := RepresentativeArg("write_file", json.RawMessage(`{"path":"x"}`)); got != "" {
		t.Errorf("non-exec tool arg = %q, want empty", got)
	}
}

func TestDeriveGrantRule(t *testing.T) {
	if r := DeriveGrantRule("shell", "git push origin main"); r.String() != "shell(git *)" {
		t.Errorf("exec grant = %q, want shell(git *)", r.String())
	}
	// Env-assignment prefix is skipped when finding the command head.
	if r := DeriveGrantRule("shell", "GIT_PAGER=cat git log"); r.String() != "shell(git *)" {
		t.Errorf("env-prefixed grant = %q, want shell(git *)", r.String())
	}
	// Non-exec tools fall back to a whole-tool grant.
	if r := DeriveGrantRule("write_file", ""); r.String() != "write_file" {
		t.Errorf("non-exec grant = %q, want write_file", r.String())
	}
}

func TestGrantsMatchesRuleAndWholeTool(t *testing.T) {
	g := NewPermissionGrants()
	g.GrantRule(DeriveGrantRule("shell", "git status"))
	if !g.Matches("shell", "git push") {
		t.Error("granted shell(git *) should match a later git command")
	}
	if g.Matches("shell", "rm -rf /") {
		t.Error("shell(git *) must not cover rm")
	}
	// Whole-tool grant matches any argument.
	g.Grant("write_file")
	if !g.Matches("write_file", "") || !g.Granted("write_file") {
		t.Error("whole-tool grant should match")
	}
}
