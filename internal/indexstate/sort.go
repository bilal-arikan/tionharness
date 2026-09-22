package indexstate

import (
	"cmp"
	"slices"
)

// sortEntries orders entries by tool then root, so repeated calls to List
// produce the same sequence. Map iteration order in Go is randomised per range,
// which would otherwise make the API response — and any UI list built from it —
// reshuffle on every poll.
func sortEntries(out []Entry) {
	slices.SortFunc(out, func(a, b Entry) int {
		if c := cmp.Compare(a.Tool, b.Tool); c != 0 {
			return c
		}
		return cmp.Compare(a.Root, b.Root)
	})
}
