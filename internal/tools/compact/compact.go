// Package compact implements System A of TionSwarm's tool-output token
// optimization: a dependency-free, deterministic compactor inspired by
// rtk-ai/rtk ("Rust Token Killer"). It shrinks a tool's combined output before
// it re-enters the LLM context by collapsing repeated lines, trimming blank
// runs, and truncating the middle of over-long output while preserving the head
// and tail (where the signal usually lives).
//
// It is intentionally cheap (no model call) and command-agnostic; the optional
// LLM intent-aware summary (System B) lives in the agent package and runs after
// this when output is still large.
package compact

import (
	"fmt"
	"strings"
)

// Options configures a single Compact pass. The zero value performs no work
// (Enabled is false); callers populate it from settings/tunables.
type Options struct {
	Enabled  bool // master switch for System A
	Dedupe   bool // collapse consecutive identical lines into "line (×N)"
	MaxLines int  // cap line count (0 = unlimited); excess is elided from the middle
	MaxBytes int  // cap byte length (0 = unlimited); applied after line work
}

// Stats reports what a Compact pass did, for logging and savings meters.
type Stats struct {
	BeforeBytes int
	AfterBytes  int
	Applied     bool // true when the output was actually changed
}

// Saved returns the number of bytes removed by the pass (never negative).
func (s Stats) Saved() int {
	if s.AfterBytes >= s.BeforeBytes {
		return 0
	}
	return s.BeforeBytes - s.AfterBytes
}

// Compact applies the deterministic techniques to output and returns the
// shrunken text plus stats. When opts.Enabled is false it returns the input
// untouched. It never returns an error: the worst case is the original string.
func Compact(output string, opts Options) (string, Stats) {
	st := Stats{BeforeBytes: len(output), AfterBytes: len(output)}
	if !opts.Enabled || output == "" {
		return output, st
	}

	lines := strings.Split(output, "\n")
	if opts.Dedupe {
		lines = dedupeConsecutive(lines)
	}
	lines = collapseBlankRuns(lines)
	if opts.MaxLines > 0 {
		lines = elideMiddleLines(lines, opts.MaxLines)
	}

	result := strings.Join(lines, "\n")
	if opts.MaxBytes > 0 && len(result) > opts.MaxBytes {
		result = truncateMiddleBytes(result, opts.MaxBytes)
	}

	st.AfterBytes = len(result)
	st.Applied = result != output
	return result, st
}

// dedupeConsecutive collapses runs of identical adjacent lines (ignoring
// trailing whitespace) into a single line annotated with the repeat count.
// Non-repeated lines pass through with their trailing whitespace trimmed.
func dedupeConsecutive(lines []string) []string {
	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		cur := strings.TrimRight(lines[i], " \t\r")
		j := i + 1
		for j < len(lines) && strings.TrimRight(lines[j], " \t\r") == cur {
			j++
		}
		n := j - i
		if n > 1 {
			out = append(out, fmt.Sprintf("%s  (×%d)", cur, n))
		} else {
			out = append(out, cur)
		}
		i = j
	}
	return out
}

// collapseBlankRuns reduces any run of 2+ blank lines to a single blank line.
func collapseBlankRuns(lines []string) []string {
	out := make([]string, 0, len(lines))
	prevBlank := false
	for _, ln := range lines {
		blank := strings.TrimSpace(ln) == ""
		if blank && prevBlank {
			continue
		}
		out = append(out, ln)
		prevBlank = blank
	}
	return out
}

// elideMiddleLines keeps the first and last halves of max and replaces the
// removed middle with a single marker line. The head gets the extra line when
// max is odd so the marker sits just past center.
func elideMiddleLines(lines []string, max int) []string {
	if len(lines) <= max {
		return lines
	}
	head := (max + 1) / 2
	tail := max - head
	skipped := len(lines) - head - tail
	out := make([]string, 0, max+1)
	out = append(out, lines[:head]...)
	out = append(out, fmt.Sprintf("… %d satır atlandı …", skipped))
	out = append(out, lines[len(lines)-tail:]...)
	return out
}

// truncateMiddleBytes hard-caps result to roughly max bytes by keeping a head
// and tail slice and inserting a marker between them. It cuts on rune
// boundaries so the output stays valid UTF-8.
func truncateMiddleBytes(s string, max int) string {
	const marker = "\n… [çıktı %d bayt kırpıldı] …\n"
	// Reserve room for the marker; if max is tiny just keep a head slice.
	budget := max - len(fmt.Sprintf(marker, 0)) - 8
	if budget < 16 {
		return cutRunes(s, max)
	}
	head := budget * 2 / 3
	tail := budget - head
	cutBytes := len(s) - head - tail
	if cutBytes <= 0 {
		return s
	}
	return cutRunes(s, head) + fmt.Sprintf(marker, cutBytes) + cutRunesTail(s, tail)
}

// cutRunes returns the longest prefix of s that fits in n bytes without
// splitting a multi-byte rune.
func cutRunes(s string, n int) string {
	if n >= len(s) {
		return s
	}
	for n > 0 && !utf8RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// cutRunesTail returns the longest suffix of s that fits in n bytes without
// splitting a multi-byte rune.
func cutRunesTail(s string, n int) string {
	if n >= len(s) {
		return s
	}
	start := len(s) - n
	for start < len(s) && !utf8RuneStart(s[start]) {
		start++
	}
	return s[start:]
}

// utf8RuneStart reports whether b is the first byte of a UTF-8 rune (i.e. not a
// 0b10xxxxxx continuation byte).
func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }
