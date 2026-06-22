package tools

import (
	"encoding/json"
	"testing"
)

func TestParsePermRule(t *testing.T) {
	cases := map[string]PermRule{
		"Bash":           {Tool: "Bash"},
		"Bash(git *)":    {Tool: "Bash", ArgGlob: "git *"},
		"  Bash( npm * )": {Tool: "Bash", ArgGlob: "npm *"},
		"Write":      {Tool: "Write"},
		"broken(":         {Tool: "broken("}, // no trailing ) → whole-tool, left as-is
	}
	for in, want := range cases {
		if got := ParsePermRule(in); got != want {
			t.Errorf("ParsePermRule(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestPermRuleMatch(t *testing.T) {
	whole := PermRule{Tool: "Bash"}
	if !whole.Match("Bash", "anything") {
		t.Error("whole-tool rule should match any argument")
	}
	if whole.Match("Write", "") {
		t.Error("rule must not match a different tool")
	}
	git := PermRule{Tool: "Bash", ArgGlob: "git *"}
	if !git.Match("Bash", "git status -s") {
		t.Error("git * should match 'git status -s'")
	}
	if git.Match("Bash", "rm -rf /") {
		t.Error("git * must not match 'rm -rf /'")
	}
	if git.Match("Write", "git status") {
		t.Error("rule is scoped to the Bash tool, not Write")
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
	if got := RepresentativeArg("Bash", cmd); got != "git status" {
		t.Errorf("Bash command arg = %q, want %q", got, "git status")
	}
	if got := RepresentativeArg("Bash", json.RawMessage(`{"command":" ls -la "}`)); got != "ls -la" {
		t.Errorf("Bash command arg = %q, want trimmed %q", got, "ls -la")
	}
	// Non-exec tools never expose an argument (only whole-tool grants apply).
	if got := RepresentativeArg("Write", json.RawMessage(`{"path":"x"}`)); got != "" {
		t.Errorf("non-exec tool arg = %q, want empty", got)
	}
}

func TestDeriveGrantRule(t *testing.T) {
	if r := DeriveGrantRule("Bash", "git push origin main"); r.String() != "Bash(git *)" {
		t.Errorf("exec grant = %q, want Bash(git *)", r.String())
	}
	// Env-assignment prefix is skipped when finding the command head.
	if r := DeriveGrantRule("Bash", "GIT_PAGER=cat git log"); r.String() != "Bash(git *)" {
		t.Errorf("env-prefixed grant = %q, want Bash(git *)", r.String())
	}
	// Non-exec tools fall back to a whole-tool grant.
	if r := DeriveGrantRule("Write", ""); r.String() != "Write" {
		t.Errorf("non-exec grant = %q, want Write", r.String())
	}
}

func TestGrantsMatchesRuleAndWholeTool(t *testing.T) {
	g := NewPermissionGrants()
	g.GrantRule(DeriveGrantRule("Bash", "git status"))
	if !g.Matches("Bash", "git push") {
		t.Error("granted Bash(git *) should match a later git command")
	}
	if g.Matches("Bash", "rm -rf /") {
		t.Error("Bash(git *) must not cover rm")
	}
	// Whole-tool grant matches any argument.
	g.Grant("Write")
	if !g.Matches("Write", "") || !g.Granted("Write") {
		t.Error("whole-tool grant should match")
	}
}
