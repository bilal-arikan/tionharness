package api

import (
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/textutil"
)

// sessionSearchNeedle folds the sidebar's free-text query (?q= on the session
// list) for sessionMatchesSearch. Empty when the query is blank, which disables
// the filter.
func sessionSearchNeedle(raw string) string {
	return textutil.FoldLower(strings.TrimSpace(raw))
}

// sessionMatchesSearch is the server twin of the sidebar's local title/id
// filter: a case-insensitive substring match over the title and the session
// id. Applying it before paging is what lets a search reach sessions the
// sidebar has not loaded yet, instead of only the rows already on screen.
func sessionMatchesSearch(s db.Session, needle string) bool {
	if needle == "" {
		return true
	}
	return textutil.ContainsFold(s.Title, needle) || textutil.ContainsFold(s.ID, needle)
}
