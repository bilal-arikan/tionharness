package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncBytesNeverSplitsARune(t *testing.T) {
	s := strings.Repeat("ğüşıöç", 40)
	for n := 0; n <= len(s)+2; n++ {
		got := TruncBytes(s, n)
		if !utf8.ValidString(got) {
			t.Fatalf("TruncBytes(s, %d) invalid UTF-8", n)
		}
		if len(got) > n && len(s) > n {
			t.Fatalf("TruncBytes(s, %d) returned %d bytes", n, len(got))
		}
		if tail := TailBytes(s, n); !utf8.ValidString(tail) {
			t.Fatalf("TailBytes(s, %d) invalid UTF-8", n)
		}
	}
}

func TestTruncBytesEllipsisOnlyWhenCut(t *testing.T) {
	if got := TruncBytesEllipsis("abc", 10); got != "abc" {
		t.Fatalf("short string changed: %q", got)
	}
	got := TruncBytesEllipsis("ğüş", 3)
	if !strings.HasSuffix(got, "…") || !utf8.ValidString(got) {
		t.Fatalf("cut string = %q", got)
	}
}

func TestTailBytesKeepsTheEnd(t *testing.T) {
	if got := TailBytes("abcdef", 3); got != "def" {
		t.Fatalf("TailBytes = %q, want def", got)
	}
	if got := TailBytes("abc", 10); got != "abc" {
		t.Fatalf("TailBytes short = %q", got)
	}
}
