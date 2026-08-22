package insight

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A byte-slicing truncate splits a multi-byte rune and the resulting prompt is
// rejected by the codex CLI as invalid UTF-8.
func TestTruncateKeepsValidUTF8(t *testing.T) {
	s := strings.Repeat("ğüşıöç", 50)
	for n := 1; n < len(s); n++ {
		got := truncate(s, n)
		if !utf8.ValidString(got) {
			t.Fatalf("truncate(s, %d) produced invalid UTF-8", n)
		}
		if len(got) > n+len("…") {
			t.Fatalf("truncate(s, %d) returned %d bytes", n, len(got))
		}
	}
	if truncate("abc", 10) != "abc" {
		t.Fatal("short string must pass through unchanged")
	}
}
