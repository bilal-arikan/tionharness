package providers

import (
	"strings"
	"testing"
)

func TestCapToolOutputPassesSmallOutput(t *testing.T) {
	in := "line one\nline two\n"
	if got := CapToolOutput(in); got != in {
		t.Fatalf("small output was modified: %q", got)
	}
}

// The real regression: few lines, each one an entire serialized record. A
// byte-total cap alone would keep the first line whole (still ~50KB of noise),
// so the per-line cap has to fire too.
func TestCapToolOutputCapsHugeLines(t *testing.T) {
	huge := strings.Repeat("x", 50*1024)
	in := strings.Repeat(huge+"\n", 20)
	got := CapToolOutput(in)
	if len(got) > traceOutputMaxBytes+128 {
		t.Fatalf("output not bounded: %d bytes", len(got))
	}
	if !strings.Contains(got, "line truncated at 4KB") {
		t.Fatal("missing per-line truncation marker")
	}
	if !strings.Contains(got, "[output truncated at 64KB]") {
		t.Fatal("missing total truncation marker")
	}
}

func TestCapToolOutputCapsManyShortLines(t *testing.T) {
	in := strings.Repeat("short line\n", 20000)
	got := CapToolOutput(in)
	if len(got) > traceOutputMaxBytes+128 {
		t.Fatalf("output not bounded: %d bytes", len(got))
	}
	if strings.Contains(got, "line truncated") {
		t.Fatal("short lines must not be line-truncated")
	}
}

func TestCapToolOutputSingleLineNoNewline(t *testing.T) {
	got := CapToolOutput(strings.Repeat("y", 200*1024))
	if len(got) > traceOutputMaxLineBytes+128 {
		t.Fatalf("single line not bounded: %d bytes", len(got))
	}
}

func TestCapToolOutputKeepsRunesIntact(t *testing.T) {
	// A multi-byte rune straddling the cut point must not be split in half.
	in := strings.Repeat("ş", traceOutputMaxLineBytes) // 2 bytes each
	got := capLines(in)
	head := strings.TrimSuffix(got, "…[line truncated at 4KB]")
	if strings.ContainsRune(head, '�') {
		t.Fatal("truncation split a rune")
	}
}
