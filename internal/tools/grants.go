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
	rules   []PermRule // argument-aware grants (B2), e.g. shell(git *)
}

// NewPermissionGrants creates an empty grant set.
func NewPermissionGrants() *PermissionGrants {
	return &PermissionGrants{allowed: map[string]bool{}}
}

// Granted reports whether tool has a standing whole-tool "Always allow" grant.
// Nil-safe. Prefer Matches when an argument is available so argument-scoped
// grants (shell(git *)) are honoured too.
func (g *PermissionGrants) Granted(tool string) bool {
	return g.Matches(tool, "")
}

// Matches reports whether a standing grant covers a call to tool with the given
// representative argument: either a whole-tool grant, or an argument-scoped rule
// whose glob matches arg. Nil-safe.
func (g *PermissionGrants) Matches(tool, arg string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.allowed[tool] {
		return true
	}
	for _, r := range g.rules {
		if r.Match(tool, arg) {
			return true
		}
	}
	return false
}

// Grant records a standing whole-tool "Always allow" for tool. Nil-safe.
func (g *PermissionGrants) Grant(tool string) {
	g.GrantRule(PermRule{Tool: tool})
}

// GrantRule records a standing "Always allow" rule — either whole-tool (empty
// ArgGlob) or argument-scoped (e.g. shell(git *)). Whole-tool rules collapse
// into the fast-path map; argument-scoped rules are de-duplicated. Nil-safe.
func (g *PermissionGrants) GrantRule(r PermRule) {
	if g == nil || r.Tool == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if r.ArgGlob == "" {
		g.allowed[r.Tool] = true
		return
	}
	for _, ex := range g.rules {
		if ex == r {
			return
		}
	}
	g.rules = append(g.rules, r)
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
