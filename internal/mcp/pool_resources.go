package mcp

import (
	"context"
	"errors"
	"fmt"
)

// ServerResources is one server's answer to a resource listing. It is a value,
// not an error/result pair, because "list everything" must report on EVERY
// server: a failure on one is part of the answer, not a reason to discard the
// other four (the same shape Catalog's per-server errs map exists for).
type ServerResources struct {
	Server    string     // server name as configured
	Resources []Resource // concrete entries first, then templates
	// Unsupported is true when the server never advertised
	// capabilities.resources. Distinct from Err: nothing went wrong, the server
	// simply has no resource surface, and saying so is more useful than an empty
	// list that looks like "it has none".
	Unsupported bool
	// Err is a hard failure for this server (not connected, resources/list
	// failed, malformed payload). Resources is then empty.
	Err string
	// Note is a NON-fatal problem: today, a resources/templates/list failure that
	// left the concrete resources intact. Reported so the absence of templates is
	// never silently indistinguishable from a server that has none.
	Note string
}

// Resources lists the resources of every given server over its pooled
// connection, one entry per config in the input order. Each server is dialed
// through the SAME slot Catalog and Call use (scoped when cfg.ScopeKey is set),
// so a scoped server is read on the caller's own connection.
//
// It never returns an error: every failure mode is per-server and reported in
// the result, because one unreachable server must not erase the listing of the
// others.
func (p *Pool) Resources(ctx context.Context, cfgs []ServerConfig) []ServerResources {
	out := make([]ServerResources, 0, len(cfgs))
	for _, cfg := range cfgs {
		out = append(out, p.resourcesOne(ctx, cfg))
	}
	return out
}

// resourcesOne lists one server's resources, converting every failure into a
// described outcome.
func (p *Pool) resourcesOne(ctx context.Context, cfg ServerConfig) ServerResources {
	res := ServerResources{Server: cfg.Name}
	name, _, _ := SplitNamespaced(NamespaceTool(cfg.Name, "x"))
	e := p.entry(scopedEntryKey(cfg.ScopeKey, name))
	e.mu.Lock()
	client, err := p.ensure(ctx, e, cfg)
	e.mu.Unlock()
	if err != nil {
		res.Err = err.Error()
		return res
	}
	if !client.SupportsResources() {
		res.Unsupported = true
		return res
	}
	list, err := client.ListResources(ctx)
	switch {
	case err == nil:
	case errors.Is(err, ErrResourcesUnsupported):
		// The capability check above already covers this; keep the branch so a
		// transport that decides late still reports "unsupported" rather than a
		// failure the operator would go debugging.
		res.Unsupported = true
		return res
	case len(list) > 0:
		// Partial success: resources/list worked, templates did not. Keep the
		// entries and carry the reason (see ServerResources.Note).
		res.Note = err.Error()
	default:
		res.Err = err.Error()
		return res
	}
	res.Resources = list
	return res
}

// ReadResource reads one resource from one server over its pooled connection.
// Unlike Resources this DOES return an error: the caller named a single server
// and URI, so a failure is the whole answer and must not be softened into an
// empty success.
func (p *Pool) ReadResource(ctx context.Context, cfg ServerConfig, uri string) ([]ResourceContent, error) {
	name, _, _ := SplitNamespaced(NamespaceTool(cfg.Name, "x"))
	e := p.entry(scopedEntryKey(cfg.ScopeKey, name))
	e.mu.Lock()
	client, err := p.ensure(ctx, e, cfg)
	e.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("mcp %q: not connected: %w", cfg.Name, err)
	}
	if !client.SupportsResources() {
		return nil, fmt.Errorf("mcp %q: %w", cfg.Name, ErrResourcesUnsupported)
	}
	contents, err := client.ReadResource(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("mcp %q: reading %s: %w", cfg.Name, uri, err)
	}
	return contents, nil
}
