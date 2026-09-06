package textutil

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// FoldLower lowers every rune of s with unicode.ToLower. Unlike strings.ToLower
// it maps the Turkish dotted capital 'İ' to a plain 'i' (strings.ToLower yields
// "i̇", an 'i' plus a combining dot), so a query typed as "İstanbul" and a text
// containing "istanbul" fold onto the same bytes. Needles handed to IndexFold,
// ContainsFold and CountFold must be folded with this function.
func FoldLower(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf || ('A' <= c && c <= 'Z') {
			return strings.Map(foldRune, s)
		}
	}
	return s
}

func foldRune(r rune) rune {
	if r < utf8.RuneSelf {
		if 'A' <= r && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}
	return unicode.ToLower(r)
}

// IndexFold returns the byte index in hay of the first case-insensitive
// occurrence of needle, or -1. hay is folded rune by rune on the fly, so no
// lower-cased copy of it is allocated: the scan of a large transcript costs
// nothing beyond the comparison itself. needle must already be FoldLower-ed.
// An empty needle matches at 0.
func IndexFold(hay, needle string) int {
	idx, _ := indexFold(hay, needle)
	return idx
}

// ContainsFold reports whether needle occurs in hay case-insensitively (see
// IndexFold for the folding contract).
func ContainsFold(hay, needle string) bool {
	return IndexFold(hay, needle) >= 0
}

// CountFold counts the non-overlapping case-insensitive occurrences of needle
// in hay. An empty needle counts as zero occurrences.
func CountFold(hay, needle string) int {
	if needle == "" {
		return 0
	}
	n := 0
	for {
		idx, width := indexFold(hay, needle)
		if idx < 0 {
			return n
		}
		n++
		hay = hay[idx+width:]
	}
}

// indexFold returns the byte index of the first match and the byte width of
// the matched hay segment (the two can differ: 'İ' is two bytes, 'i' one).
func indexFold(hay, needle string) (int, int) {
	if needle == "" {
		return 0, 0
	}
	first, firstWidth := utf8.DecodeRuneInString(needle)
	rest := needle[firstWidth:]
	for i := 0; i < len(hay); {
		r, w := utf8.DecodeRuneInString(hay[i:])
		if foldRune(r) == first {
			if width, ok := matchFoldPrefix(hay[i+w:], rest); ok {
				return i, w + width
			}
		}
		i += w
	}
	return -1, 0
}

// matchFoldPrefix reports whether hay starts with needle under folding and how
// many bytes of hay the match spans.
func matchFoldPrefix(hay, needle string) (int, bool) {
	consumed := 0
	for len(needle) > 0 {
		if len(hay) == 0 {
			return 0, false
		}
		nr, nw := utf8.DecodeRuneInString(needle)
		hr, hw := utf8.DecodeRuneInString(hay)
		if foldRune(hr) != nr {
			return 0, false
		}
		needle = needle[nw:]
		hay = hay[hw:]
		consumed += hw
	}
	return consumed, true
}
