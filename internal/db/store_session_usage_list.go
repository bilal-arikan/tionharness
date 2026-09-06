package db

import "context"

// ListSessionUsage returns a copy of every session's lifetime usage rollup,
// keyed by session id. Used by the evolution fitness computation, which needs
// the whole workspace at once rather than one session at a time.
func (d *DB) ListSessionUsage(ctx context.Context) map[string]SessionUsage {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[string]SessionUsage, len(d.sessionUsage))
	for id, u := range d.sessionUsage {
		out[id] = u
	}
	return out
}

// ListSessionAsks returns every durable ask (any status), unordered.
func (d *DB) ListSessionAsks(ctx context.Context) []SessionAsk {
	return dbList(d, d.sessionAsks, nil)
}
