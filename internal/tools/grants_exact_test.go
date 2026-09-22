package tools

import "testing"

func TestMatchesExactIgnoresFamilyGrants(t *testing.T) {
	g := NewPermissionGrants()
	g.GrantRule(PermRule{Tool: "Bash", ArgGlob: "git *"})
	if g.MatchesExact("Bash", "git push --force origin main") {
		t.Error("a family glob counted as an exact approval")
	}
	g.GrantRule(ExactGrantRule("Bash", "git push --force origin main"))
	if !g.MatchesExact("Bash", "git push --force origin main") {
		t.Error("exact approval not found")
	}
	if g.MatchesExact("Bash", "git push --force origin dev") || g.MatchesExact("PowerShell", "git push --force origin main") {
		t.Error("exact approval matched a different command or tool")
	}
	var nilGrants *PermissionGrants
	if nilGrants.MatchesExact("Bash", "ls") {
		t.Error("nil grants matched")
	}
}

func TestExactGrantRuleRefusesGlobs(t *testing.T) {
	if r := ExactGrantRule("Bash", "rm -rf build/*"); r.Tool != "" {
		t.Errorf("a command with * produced a rule %v, which would act as a glob", r)
	}
	if r := ExactGrantRule("Bash", ""); r.Tool != "" {
		t.Errorf("empty command produced a rule %v", r)
	}
	if r := ExactGrantRule("Bash", "git push"); r.String() != "Bash(git push)" {
		t.Errorf("rule = %v", r)
	}
}
