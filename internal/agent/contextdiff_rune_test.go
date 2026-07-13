package agent

import (
	"strings"
	"testing"
)

// TestParagraphArea_MultiByteLineNoPanic reproduces the byte-vs-rune slice panic:
// a line built from 2-byte Turkish runes can exceed maxAreaLineLen BYTES while
// holding FEWER runes, so the old len(l)>max byte guard let string([]rune(l)[:max])
// slice past the rune slice and panic ("slice bounds out of range [:400] with
// capacity 384"). With the rune-count guard it must neither panic nor truncate a
// line that is under the rune cap.
func TestParagraphArea_MultiByteLineNoPanic(t *testing.T) {
	// 384 runes, each 2 bytes ('ş') => 768 bytes > maxAreaLineLen(400) but 384 < 400 runes.
	line := strings.Repeat("ş", 384)
	if len(line) <= maxAreaLineLen {
		t.Fatalf("test premise broken: want byte len > %d, got %d", maxAreaLineLen, len(line))
	}
	if n := len([]rune(line)); n >= maxAreaLineLen {
		t.Fatalf("test premise broken: want rune len < %d, got %d", maxAreaLineLen, n)
	}
	area := paragraphArea(line, ContextAdded) // must not panic
	if len(area.Lines) != 1 || area.Lines[0] != line {
		t.Fatalf("under-cap multi-byte line should pass through untruncated, got %q", area.Lines)
	}
}

// TestParagraphArea_TruncatesByRune ensures a genuinely over-cap line is still
// truncated to exactly maxAreaLineLen RUNES plus the ellipsis.
func TestParagraphArea_TruncatesByRune(t *testing.T) {
	line := strings.Repeat("a", maxAreaLineLen+100)
	area := paragraphArea(line, ContextAdded)
	if len(area.Lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(area.Lines))
	}
	got := []rune(area.Lines[0])
	if len(got) != maxAreaLineLen+1 || string(got[len(got)-1:]) != "…" {
		t.Fatalf("want %d runes ending in ellipsis, got %d runes: %q", maxAreaLineLen+1, len(got), area.Lines[0])
	}
}
