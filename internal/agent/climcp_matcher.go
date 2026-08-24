package agent

import "strings"

// cliMatcherRegex translates a TionHarness hook matcher into the regex Claude Code
// expects in its settings hooks block.
//
// TionHarness's NATIVE matcher (hookMatches) is a COMMA-separated list of
// filepath.Match GLOBS, full-matched against the tool name ("Bash,PowerShell" or
// "http_*"). Claude Code instead treats a matcher as a REGEX (alternation is `|`,
// not `,`, and it is unanchored). Passing the comma list verbatim therefore makes
// the CLI search for the literal string "Bash,PowerShell,…" — which no single tool
// name contains, so the hook SILENTLY NEVER FIRES on claude-cli-delegated turns
// (the path most agents use). rtk/sqz optimizers wired with such matchers were dead
// in the CLI path; this bridges the two dialects.
//
// Conversion: split on ",", turn each glob into a regex fragment (* -> .*, ? -> .,
// other regex metachars escaped), join with "|", and ANCHOR the whole with ^...$ so
// CLI matching mirrors native filepath.Match's full-string semantics (so "Bash"
// does not also match "BashOutput"). An empty matcher stays empty — both engines
// treat that as "every tool".
//
// Bridged-shell expansion: when the built-in shell is on, the CLI does NOT see a
// plain "Bash"/"PowerShell" tool — it sees the interaction-server-namespaced
// mcp__tionharness_interaction__Bash / __PowerShell. So a matcher naming the plain
// shell tool would still miss every shell call on a claude-cli turn. We therefore
// auto-add the bridged form for those two names, deduped — a plain "Bash,PowerShell"
// matcher now fires across every workspace with no per-workspace data edit. Native
// (non-bridged) turns are unaffected: the bridged alternative simply never matches
// a tool that isn't present.
var shellBridgedForms = map[string]string{
	"Bash":       interactionToolPrefix + "Bash",
	"PowerShell": interactionToolPrefix + "PowerShell",
}

func cliMatcherRegex(matcher string) string {
	m := strings.TrimSpace(matcher)
	if m == "" {
		return ""
	}
	var alts []string
	seen := map[string]bool{}
	add := func(frag string) {
		if frag == "" || seen[frag] {
			return
		}
		seen[frag] = true
		alts = append(alts, frag)
	}
	for _, part := range strings.Split(m, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		add(globToRegexFragment(part))
		if bridged, ok := shellBridgedForms[part]; ok {
			add(globToRegexFragment(bridged))
		}
	}
	switch len(alts) {
	case 0:
		return ""
	case 1:
		return "^" + alts[0] + "$"
	default:
		return "^(" + strings.Join(alts, "|") + ")$"
	}
}

// globToRegexFragment converts one filepath.Match-style glob to an equivalent regex
// fragment: * -> .*, ? -> ., every other regex-special rune escaped so tool names
// (including MCP's "mcp__server__tool") match literally.
func globToRegexFragment(glob string) string {
	var b strings.Builder
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '.', '+', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
