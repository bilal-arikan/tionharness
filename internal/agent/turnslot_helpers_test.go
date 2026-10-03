package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// claimSessionTurnSlot holds a fixture slot without resetting autonomous budgets.
func (r *Runtime) claimSessionTurnSlot(sessionID string, kind turnqueue.Kind, label string) func() {
	release, _ := r.claimSessionTurnSlotCtx(context.Background(), sessionID, kind, label)
	return release
}
