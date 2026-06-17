package tools

import (
	"strings"
	"testing"
)

func TestCapToolOutput(t *testing.T) {
	// Under the cap: returned verbatim.
	small := "hello world"
	if got := capToolOutput(small); got != small {
		t.Fatalf("small output altered: %q", got)
	}

	// Over the cap: truncated with a marker, never longer than cap + marker.
	big := strings.Repeat("a", maxToolOutputBytes+5000)
	got := capToolOutput(big)
	if len(got) >= len(big) {
		t.Fatalf("oversized output not truncated: len=%d", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("missing truncation marker: %q", got[len(got)-40:])
	}
}

func TestCapToolOutputRuneBoundary(t *testing.T) {
	// A multibyte rune straddling the cut point must not produce invalid UTF-8.
	body := strings.Repeat("é", maxToolOutputBytes) // 2 bytes each → way over cap
	got := capToolOutput(body)
	marker := strings.Index(got, "\n…[truncated")
	if marker < 0 {
		t.Fatalf("expected truncation marker")
	}
	if !strings.HasPrefix(body, got[:marker]) {
		t.Fatalf("truncated prefix is not a valid prefix of the input")
	}
}
