package agent

import (
	"context"

	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// activeToolsKey carries the per-turn lazy-tool active set through the context,
// so buildRegistry can wire the activate_tools/deactivate_tools meta-tools to
// the same set the tool loop reads when assembling each request. It is unset for
// catalog/preview calls (then the meta-tools are nil-safe no-ops).
type activeToolsKey struct{}

// withActiveTools attaches the turn's active set to ctx.
func withActiveTools(ctx context.Context, a *tools.ActiveTools) context.Context {
	return context.WithValue(ctx, activeToolsKey{}, a)
}

// activeToolsFromCtx returns the turn's active set, or nil when none is set.
func activeToolsFromCtx(ctx context.Context) *tools.ActiveTools {
	a, _ := ctx.Value(activeToolsKey{}).(*tools.ActiveTools)
	return a
}
