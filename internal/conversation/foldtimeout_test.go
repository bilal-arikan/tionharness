package conversation

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestFoldCtxRaisesIdleOutputFloor: a fold's provider call must carry the longer
// stdout-silence budget, otherwise the per-turn watchdog kills /handoff and
// /compact while the model is still producing its first token.
func TestFoldCtxRaisesIdleOutputFloor(t *testing.T) {
	if got := providers.IdleOutputFloor(foldCtx(context.Background())); got != FoldIdleOutputFloor {
		t.Fatalf("fold idle floor = %s, want %s", got, FoldIdleOutputFloor)
	}
}
