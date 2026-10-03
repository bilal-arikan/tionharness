package textutil

import "strings"

// TruncRunesEllipsis caps s to max runes, trimming the cut prefix before
// appending an ellipsis. Text within the limit is returned unchanged.
func TruncRunesEllipsis(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
}
