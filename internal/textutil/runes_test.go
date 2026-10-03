package textutil

import (
	"testing"
	"unicode/utf8"
)

func TestTruncRunesEllipsisPreservesTextAndUnicode(t *testing.T) {
	cases := []struct {
		name, input, want string
		max               int
	}{
		{"empty", "", "", 0},
		{"zero budget", "word", "…", 0},
		{"short whitespace", " word \t", " word \t", 8},
		{"exact boundary", "ğ😀ç", "ğ😀ç", 3},
		{"Unicode cut", "ğ😀ç", "ğ😀…", 2},
		{"trim cut prefix", " \tğ \tend", "ğ…", 5},
		{"all whitespace prefix", "   word", "…", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TruncRunesEllipsis(tc.input, tc.max)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("invalid UTF-8: %q", got)
			}
		})
	}
}
