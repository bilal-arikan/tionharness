package tools

import (
	"context"
	"sync"
)

// PermissionGrants records the tools a user chose "Always allow" for, scoped to
// a single chat session, so the permission gate (native loop and the CLI
// permission-prompt tool) does not re-prompt for the same tool across turns.
// In-memory: grants last for the session's active life (reset on server restart).
type PermissionGrants struct {
	mu      sync.Mutex
	allowed map[string]bool
}

// NewPermissionGrants creates an empty grant set.
func NewPermissionGrants() *PermissionGrants {
	return &PermissionGrants{allowed: map[string]bool{}}
}

// Granted reports whether tool has a standing "Always allow" grant. Nil-safe.
func (g *PermissionGrants) Granted(tool string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.allowed[tool]
}

// Grant records a standing "Always allow" for tool. Nil-safe.
func (g *PermissionGrants) Grant(tool string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.allowed[tool] = true
}

// grantsKey keys the session grants on a request context.
type grantsKey struct{}

// WithGrants attaches the session's permission grants to ctx so the native gate
// can honour "Always allow" across turns.
func WithGrants(ctx context.Context, g *PermissionGrants) context.Context {
	return context.WithValue(ctx, grantsKey{}, g)
}

// GrantsFrom returns the grants attached to ctx, or nil when none is present.
func GrantsFrom(ctx context.Context) *PermissionGrants {
	g, _ := ctx.Value(grantsKey{}).(*PermissionGrants)
	return g
}
