package mcp

import (
	"context"
	"sync"
)

// EnsureOutcome is the per-server result of an EnsureServers warm-up: what the
// pool now knows about that server's LIVE connection, plus the tools it listed.
//
// State is the same tri-state the Stats/ServerState pair reports, but resolved
// from THIS dial rather than from a snapshot: after EnsureServers a server is
// either ServerAlive (handshake + tools/list succeeded) or ServerDead (Err says
// why). ServerUnknown never comes out of here — we always dialed.
type EnsureOutcome struct {
	Server string      // server name as configured
	State  ServerState // ServerAlive or ServerDead
	Tools  []Tool      // listed tools (nil when the server is dead)
	Err    string      // dial/list failure reason; empty when alive
}

// EnsureServers dials every given server IN PARALLEL and waits for all of them
// to settle, returning one outcome per config in the input order.
//
// Parallelism is the point. Catalog dials serially, which is right for a catalog
// build that is already amortised across a turn, but a caller that deliberately
// blocks on N servers cannot pay N × DefaultDialTimeout — five unreachable
// servers would be 100 seconds of a wedged turn. Here the wall-clock cost of the
// whole set is that of the SLOWEST server, and ctx bounds the set as a whole:
// cancel it and every straggler is abandoned at once.
//
// Each server still gets its own pool slot (scoped when cfg.ScopeKey is set), so
// a warmed connection is the SAME one a later Catalog/Call reuses — that is what
// makes this a warm-up rather than a probe. Failures are reported, never
// returned as an error: a caller asking "are these up?" wants the per-server
// verdict, and one dead server must not erase the news about four live ones.
func (p *Pool) EnsureServers(ctx context.Context, cfgs []ServerConfig) []EnsureOutcome {
	out := make([]EnsureOutcome, len(cfgs))
	var wg sync.WaitGroup
	for i, cfg := range cfgs {
		wg.Add(1)
		go func(i int, cfg ServerConfig) {
			defer wg.Done()
			out[i] = p.ensureOne(ctx, cfg)
		}(i, cfg)
	}
	wg.Wait()
	return out
}

// ensureOne dials a single server and lists its tools, reusing the pool slot
// Catalog would use. Runs on its own goroutine under EnsureServers.
func (p *Pool) ensureOne(ctx context.Context, cfg ServerConfig) EnsureOutcome {
	name, _, _ := SplitNamespaced(NamespaceTool(cfg.Name, "x"))
	e := p.entry(scopedEntryKey(cfg.ScopeKey, name))
	e.mu.Lock()
	list, err := p.tools(ctx, e, cfg)
	e.mu.Unlock()
	if err != nil {
		return EnsureOutcome{Server: cfg.Name, State: ServerDead, Err: err.Error()}
	}
	return EnsureOutcome{Server: cfg.Name, State: ServerAlive, Tools: list}
}

// CatalogEntries converts the ALIVE outcomes into namespaced catalog entries,
// in the same shape Catalog produces, so a caller can merge freshly warmed tools
// into a live registry without rebuilding the whole catalog.
func CatalogEntries(outcomes []EnsureOutcome) []CatalogEntry {
	var entries []CatalogEntry
	for _, o := range outcomes {
		if o.State != ServerAlive {
			continue
		}
		for _, t := range o.Tools {
			entries = append(entries, CatalogEntry{
				Server:         o.Server,
				NamespacedName: NamespaceTool(o.Server, t.Name),
				Tool:           t,
			})
		}
	}
	return entries
}
