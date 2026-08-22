// Package textutil holds the byte-safe string cuts shared by every layer that
// caps text before it reaches a prompt, a tool result or a log line.
//
// The rule these functions exist to enforce: a plain s[:n] splits a multi-byte
// rune. The half rune serializes as U+FFFD in a preview, and in a CLI prompt it
// is fatal — codex rejects the whole turn with "input is not valid UTF-8
// (invalid byte at offset N)", which is how a Turkish transcript silently killed
// every insight lens. Cut on a rune boundary instead.
package textutil

import "unicode/utf8"

// TruncBytes returns the longest prefix of s that fits in n bytes without
// splitting a rune. It appends nothing — callers that want an ellipsis add their
// own, so the byte cap stays exactly n.
func TruncBytes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// TruncBytesEllipsis is TruncBytes plus a trailing "…" when anything was cut.
// The ellipsis is added beyond the n-byte budget, matching the callers that
// treat n as "how much content", not "how many bytes on the wire".
func TruncBytesEllipsis(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return TruncBytes(s, n) + "…"
}

// TailBytes returns the last n bytes of s, starting on a rune boundary. Used by
// diagnostics that keep the END of a long output (the error is at the bottom).
func TailBytes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	start := len(s) - n
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}
