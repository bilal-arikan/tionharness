package tools

import (
	"encoding/json"
	"strings"
)

// PermRule is a parsed permission pattern: a tool name plus an optional argument
// glob. "Bash" matches any shell call; "Bash(git *)" matches only shell calls
// whose representative argument matches the glob "git *". It is the unit behind
// argument-aware permission grants (B2): approving `git status` once with
// "always" grants Bash(git *), so later git commands skip the prompt while
// `rm -rf /` still asks.
type PermRule struct {
	Tool    string // tool name, e.g. "Bash"
	ArgGlob string // "" = match any argument (whole-tool grant)
}

// ParsePermRule parses "Tool" or "Tool(argGlob)" into a PermRule. Whitespace is
// trimmed; text without a trailing ")" is treated as a whole-tool rule.
func ParsePermRule(s string) PermRule {
	s = strings.TrimSpace(s)
	open := strings.IndexByte(s, '(')
	if open < 0 || !strings.HasSuffix(s, ")") {
		return PermRule{Tool: s}
	}
	return PermRule{
		Tool:    strings.TrimSpace(s[:open]),
		ArgGlob: strings.TrimSpace(s[open+1 : len(s)-1]),
	}
}

// String renders the rule back to canonical "Tool" / "Tool(glob)" form.
func (r PermRule) String() string {
	if r.ArgGlob == "" {
		return r.Tool
	}
	return r.Tool + "(" + r.ArgGlob + ")"
}

// Match reports whether the rule covers a call to tool with the given
// representative argument. A whole-tool rule (empty ArgGlob) matches any
// argument; otherwise the argument must match the glob.
func (r PermRule) Match(tool, arg string) bool {
	if r.Tool != tool {
		return false
	}
	if r.ArgGlob == "" {
		return true
	}
	return globMatch(r.ArgGlob, arg)
}

// globMatch reports whether s matches a glob where "*" stands for any run of
// characters (including empty). Matching is case-sensitive and anchored at both
// ends; no other metacharacters are special.
func globMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s // no wildcard → exact match
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for _, seg := range parts[1 : len(parts)-1] {
		idx := strings.Index(s, seg)
		if idx < 0 {
			return false
		}
		s = s[idx+len(seg):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// execArgTools are the tools whose risk depends on their argument (a command),
// so the gate can derive an argument-scoped grant instead of a whole-tool one.
// "Bash" is the shared exec built-in name (TionSwarm's native shell and the
// claude-cli analog both report it).
var execArgTools = map[string]bool{"Bash": true}

// RepresentativeArg returns the argument string a permission rule matches
// against for a tool call. For command-execution tools it is the command line;
// for any other tool it is "" (so only whole-tool grants apply).
func RepresentativeArg(tool string, input json.RawMessage) string {
	if !execArgTools[tool] || len(input) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return ""
	}
	for _, k := range []string{"command", "cmd", "script"} {
		if v, ok := obj[k].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// DeriveGrantRule builds the rule recorded when the user picks "Always allow"
// for a call. For a command-execution tool with a parseable command it scopes
// the grant to the command's leading word ("git status -s" → shell(git *)), so
// "always" authorises similar commands without blanket-approving the tool. For
// everything else it is a whole-tool grant (the historical behaviour).
func DeriveGrantRule(tool, arg string) PermRule {
	if execArgTools[tool] && arg != "" {
		if head := commandHead(arg); head != "" {
			return PermRule{Tool: tool, ArgGlob: head + " *"}
		}
	}
	return PermRule{Tool: tool}
}

// commandHead returns the leading command word of a shell command, skipping
// leading env-assignments so "GIT_PAGER=cat git log" → "git". Returns "" when
// nothing usable is found.
func commandHead(cmd string) string {
	for _, f := range strings.Fields(cmd) {
		if strings.Contains(f, "=") && !strings.HasPrefix(f, "-") {
			continue // VAR=value prefix
		}
		return f
	}
	return ""
}
