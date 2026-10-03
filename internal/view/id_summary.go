package view

import (
	"fmt"
	"strings"
)

// idSummary lists the first max IDs in source order and reports the remainder.
func idSummary(total, max int, idAt func(int) string) string {
	ids := make([]string, 0, max)
	for i := 0; i < total; i++ {
		if i >= max {
			return strings.Join(ids, ", ") + fmt.Sprintf(" +%d", total-max)
		}
		ids = append(ids, idAt(i))
	}
	return strings.Join(ids, ", ")
}
