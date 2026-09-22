package tools

import "strings"

// MatchesExact reports whether a standing rule was granted for exactly this
// argument (Bash(git push --force origin main)), as opposed to a family glob
// (Bash(git *)) or a whole-tool grant. The decider's risk check trusts only
// such exact approvals: a family grant also covers the family's destructive
// variants, which is precisely what the check exists to catch. Nil-safe.
func (g *PermissionGrants) MatchesExact(tool, arg string) bool {
	if g == nil || arg == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, r := range g.rules {
		if r.Tool == tool && r.ArgGlob == arg {
			return true
		}
	}
	return false
}

// ExactGrantRule is the rule recorded when the user answers "Always allow" to a
// call the decider flagged as risky: it covers this exact command only. A
// command containing "*" cannot be expressed as an exact rule (the glob would
// widen it), so it yields an empty rule, which GrantRule ignores — the answer
// then counts as a one-time approval.
func ExactGrantRule(tool, arg string) PermRule {
	if tool == "" || arg == "" || strings.Contains(arg, "*") {
		return PermRule{}
	}
	return PermRule{Tool: tool, ArgGlob: arg}
}
