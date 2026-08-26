package providers

import (
	"strings"
	"unicode/utf8"
)

// Trace output caps. CLI-backed providers (claude-cli, codex-cli) run the tool
// loop themselves, so the harness-side shell cap in internal/tools never applies
// to what they report back: whatever the CLI printed lands verbatim in
// TraceStep.Output and is persisted in the session transcript. One unlucky grep
// over a .jsonl store has produced a single 1 MB step (20 lines, ~50 KB each),
// bloating the stored message to megabytes.
//
// Two independent limits, because either one alone is escapable:
//   - traceOutputMaxLineBytes catches "few lines, each enormous" (matching a
//     whole serialized record per line).
//   - traceOutputMaxBytes catches "many normal lines".
const (
	traceOutputMaxBytes     = 64 * 1024
	traceOutputMaxLineBytes = 4 * 1024
)

// CapToolOutput bounds a CLI-reported tool result before it enters the trace.
// Truncation is always visible: an over-long line keeps its head and gains a
// marker, and a truncated result gains a trailing note. Nothing is dropped
// silently.
func CapToolOutput(s string) string {
	if len(s) <= traceOutputMaxLineBytes && len(s) <= traceOutputMaxBytes {
		return s // fast path: nothing can be over either limit
	}
	if strings.IndexByte(s, '\n') >= 0 || len(s) > traceOutputMaxLineBytes {
		s = capLines(s)
	}
	if len(s) > traceOutputMaxBytes {
		s = truncateUTF8(s, traceOutputMaxBytes) + "\n[output truncated at 64KB]"
	}
	return s
}

// capLines truncates every individual line to traceOutputMaxLineBytes.
func capLines(s string) string {
	if len(s) <= traceOutputMaxLineBytes && strings.IndexByte(s, '\n') < 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	changed := false
	for i, ln := range lines {
		if len(ln) <= traceOutputMaxLineBytes {
			continue
		}
		lines[i] = truncateUTF8(ln, traceOutputMaxLineBytes) + "…[line truncated at 4KB]"
		changed = true
	}
	if !changed {
		return s
	}
	return strings.Join(lines, "\n")
}

// truncateUTF8 cuts s to at most n bytes without splitting a rune.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 {
		r, size := utf8.DecodeLastRuneInString(s)
		if r != utf8.RuneError || size != 1 {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
