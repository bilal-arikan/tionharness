package agent

import "context"

type catalogNoDialKey struct{}

// WithCatalogNoDial marks a context whose registry builds must not dial MCP
// servers: the MCP part of the catalog comes from the pool's already-live
// connections (mcp.Pool.CatalogCached) and a cold server simply contributes
// nothing. Read-only surfaces stamp it — the session info panel used to block
// for DefaultDialTimeout per unreachable server on the first click into a
// workspace, which read as the whole app hanging. A turn never stamps it: a
// turn must dial, so its agent actually gets the tools.
func WithCatalogNoDial(ctx context.Context) context.Context {
	return context.WithValue(ctx, catalogNoDialKey{}, true)
}

// CatalogNoDialFrom reports whether the context forbids MCP dials.
func CatalogNoDialFrom(ctx context.Context) bool {
	v, _ := ctx.Value(catalogNoDialKey{}).(bool)
	return v
}
