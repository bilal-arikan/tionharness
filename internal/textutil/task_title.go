package textutil

import "strings"

// PlaceholderTaskTitle derives an instant, single-line title from task content
// until the generated title is ready.
func PlaceholderTaskTitle(source string) string {
	source = strings.TrimSpace(source)
	if i := strings.IndexAny(source, "\r\n"); i >= 0 {
		source = strings.TrimSpace(source[:i])
	}
	return TruncRunesEllipsis(source, 60)
}
