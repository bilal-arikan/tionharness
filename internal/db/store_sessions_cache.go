package db

import "sort"

// sessionsSnapshot is the normalised, sorted session list as of one mutation
// generation. The slice is shared between callers and must never be written
// to: ListSessions hands out copies.
type sessionsSnapshot struct {
	gen  uint64
	list []Session
}

// sortedSessions returns every session pinned-first, newest-updated first,
// normalised (normalizeSessionMeta) — the order ListSessions has always used.
//
// Every list-shaped surface (sidebar paging, executions feed, dashboard,
// projector, context blocks) asks for this same list, and each request used to
// copy and re-sort the whole map under the read lock. The result is now
// memoised against MutationGen: a burst of requests between two writes shares
// one build, and a write invalidates it for free (no callback, no eviction).
func (d *DB) sortedSessions() []Session {
	if snap := d.sessionsSorted.Load(); snap != nil && snap.gen == d.mutGen.Load() {
		return snap.list
	}
	d.mu.RLock()
	// Read the generation under the same read lock as the maps: writers bump it
	// while holding the write lock, so what we build is exactly this generation.
	gen := d.mutGen.Load()
	out := make([]Session, 0, len(d.sessions))
	for _, s := range d.sessions {
		out = append(out, normalizeSessionMeta(s))
	}
	d.mu.RUnlock()
	// Pinned sessions float to the top; within each group, most-recently-updated
	// first. A view preference, so it never changes the underlying activity order.
	// Tie-break on ID so equal-UpdatedAt sessions keep a STABLE order across calls
	// (the source map iterates in random order, so without this the list reshuffles
	// on every poll).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		return out[i].ID > out[j].ID
	})
	d.sessionsSorted.Store(&sessionsSnapshot{gen: gen, list: out})
	return out
}
