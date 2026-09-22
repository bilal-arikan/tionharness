package decider

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// The state sent to a decision service leaves the machine, so obvious secrets
// are masked first. The patterns are deliberately conservative (well-known key
// formats, bearer tokens, key=value assignments to secret-looking names, private
// key blocks, credentials inside URLs): a missed secret is a leak, a masked
// ordinary word only costs the model a little context.
var secretPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), "[REDACTED PRIVATE KEY]"},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{16,}`), "[REDACTED]"},
	{regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}`), "[REDACTED]"},
	{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), "[REDACTED]"},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), "[REDACTED]"},
	{regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}`), "[REDACTED]"},
	{regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9\-]{10,}`), "[REDACTED]"},
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/\-]{16,}=*`), "Bearer [REDACTED]"},
	{regexp.MustCompile(`(?i)(\b[a-z0-9_\-]*(?:password|passwd|secret|token|api[_\-]?key|access[_\-]?key|private[_\-]?key)[a-z0-9_\-]*["']?\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s"',;]+)`), "${1}[REDACTED]"},
	{regexp.MustCompile(`(://[^/\s:@]+):[^/\s@]+@`), "${1}:[REDACTED]@"},
}

// Redact masks secrets in s.
func Redact(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s
}

// redactValue masks secrets in every string inside a JSON-like value (string,
// map, slice). Other values pass through unchanged.
func redactValue(v any) any {
	switch t := v.(type) {
	case string:
		return Redact(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = redactValue(x)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(t))
		for k, x := range t {
			out[k] = Redact(x)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = redactValue(x)
		}
		return out
	case []string:
		out := make([]string, len(t))
		for i, x := range t {
			out[i] = Redact(x)
		}
		return out
	default:
		return v
	}
}

// TrimMiddle shortens s to at most maxBytes by cutting out its middle, keeping
// the head (usually the context) and the tail (usually the latest output), with
// a marker where text was removed. The cut respects UTF-8 boundaries.
func TrimMiddle(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	const marker = "\n…[trimmed]…\n"
	if maxBytes <= len(marker)+8 {
		return truncateUTF8(s, maxBytes)
	}
	keep := maxBytes - len(marker)
	head := truncateUTF8(s, keep/3)
	tail := suffixUTF8(s, keep-len(head))
	return head + marker + tail
}

// truncateUTF8 returns the longest prefix of s of at most n bytes that ends on
// a rune boundary.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// suffixUTF8 returns the longest suffix of s of at most n bytes that starts on
// a rune boundary.
func suffixUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	i := len(s) - n
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return strings.Clone(s[i:])
}
