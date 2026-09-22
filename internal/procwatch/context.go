package procwatch

import "context"

type ownerKey struct{}

// WithOwner stamps the agent-side identity of the work being done onto the
// context, so a spawn site several layers below (a provider transport, an MCP
// client) can attribute its process without every layer between growing a
// session parameter it has no other use for.
//
// Stamp it where the identity is known — at the turn's entry point in the agent
// runtime — and let it flow. A deeper stamp overrides a shallower one only for
// the fields it sets (see mergeOwner), so a worker turn can narrow the session
// without losing the workspace.
func WithOwner(ctx context.Context, o Owner) context.Context {
	return context.WithValue(ctx, ownerKey{}, mergeOwner(OwnerFrom(ctx), o))
}

// OwnerFrom returns the identity stamped on ctx, or the zero Owner when the
// context carries none (a startup probe, a test).
func OwnerFrom(ctx context.Context) Owner {
	if ctx == nil {
		return Owner{}
	}
	if o, ok := ctx.Value(ownerKey{}).(Owner); ok {
		return o
	}
	return Owner{}
}
