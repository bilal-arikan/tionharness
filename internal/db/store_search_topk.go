package db

import (
	"container/heap"
	"sort"
)

// searchCandidate is a matched message before it is turned into a SearchHit.
// The snippet is deliberately absent: cutting a snippet (collapsing whitespace,
// converting to runes) is the expensive part of a hit, and only the top-k
// candidates ever need one.
type searchCandidate struct {
	session   *Session
	message   *Message
	score     float64
	createdAt int64
	// seq is the scan order (newest session first, chronological within it), the
	// tie-break that keeps equal-score results in the same stable order the old
	// collect-everything-then-sort implementation produced.
	seq int
}

// candidateBetter is the ranking: higher score, then newer, then earlier scan
// position.
func candidateBetter(a, b searchCandidate) bool {
	if a.score != b.score {
		return a.score > b.score
	}
	if a.createdAt != b.createdAt {
		return a.createdAt > b.createdAt
	}
	return a.seq < b.seq
}

// searchTopK keeps the best `limit` candidates seen so far in a min-heap whose
// root is the WORST retained candidate, so an incoming candidate that cannot
// beat the root is dropped in O(1) and the scan never buffers every match.
type searchTopK struct {
	limit int
	items []searchCandidate
}

func newSearchTopK(limit int) *searchTopK {
	return &searchTopK{limit: limit, items: make([]searchCandidate, 0, limit+1)}
}

func (h *searchTopK) Len() int           { return len(h.items) }
func (h *searchTopK) Less(i, j int) bool { return candidateBetter(h.items[j], h.items[i]) }
func (h *searchTopK) Swap(i, j int)      { h.items[i], h.items[j] = h.items[j], h.items[i] }
func (h *searchTopK) Push(x any)         { h.items = append(h.items, x.(searchCandidate)) }
func (h *searchTopK) Pop() any {
	n := len(h.items)
	it := h.items[n-1]
	h.items = h.items[:n-1]
	return it
}

// offer records c when it ranks among the best `limit` so far.
func (h *searchTopK) offer(c searchCandidate) {
	if len(h.items) < h.limit {
		heap.Push(h, c)
		return
	}
	if candidateBetter(c, h.items[0]) {
		h.items[0] = c
		heap.Fix(h, 0)
	}
}

// ranked returns the retained candidates best-first.
func (h *searchTopK) ranked() []searchCandidate {
	out := make([]searchCandidate, len(h.items))
	copy(out, h.items)
	sort.Slice(out, func(i, j int) bool { return candidateBetter(out[i], out[j]) })
	return out
}
