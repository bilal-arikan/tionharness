package indexstate

import "sort"

// sortEntries orders entries by tool then root, so repeated calls to List
// produce the same sequence. Map iteration order in Go is randomised per range,
// which would otherwise make the API response — and any UI list built from it —
// reshuffle on every poll.
func sortEntries(out []Entry) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tool != out[j].Tool {
			return out[i].Tool < out[j].Tool
		}
		return out[i].Root < out[j].Root
	})
}
