package textutil

import (
	"strings"
	"testing"
)

func TestPlaceholderTaskTitleContentBoundaries(t *testing.T) {
	cases := []struct {
		name, input, want string
	}{
		{"empty", "", ""},
		{"blank", " \t\r\n ", ""},
		{"trim first line", "\n  First line  \r\nSecond line", "First line"},
		{"carriage return", "First\rSecond", "First"},
		{"line feed", "First\nSecond", "First"},
		{"exact Unicode boundary", strings.Repeat("😀", 60), strings.Repeat("😀", 60)},
		{"Unicode over boundary", strings.Repeat("ğ", 61), strings.Repeat("ğ", 60) + "…"},
		{"trim before ellipsis", strings.Repeat("a", 59) + " last", strings.Repeat("a", 59) + "…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlaceholderTaskTitle(tc.input); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
