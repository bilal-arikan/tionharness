package agent

import (
	"context"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

// TestRuntimeSessionContext verifies the per-workspace session-context state on
// the Runtime: defaults off / recent falls back to the default, and a set sticks
// (and gates the list_sessions tool through buildRegistry).
func TestRuntimeSessionContext(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())

	// Unset: disabled, recent falls back to the built-in default.
	if rt.SessionContextEnabled() {
		t.Error("session context should be off until set")
	}
	if got := rt.SessionContextRecentCount(); got != DefaultSessionContextRecent {
		t.Errorf("recent fallback = %d, want %d", got, DefaultSessionContextRecent)
	}

	rt.SetSessionContext(true, true, 12)
	if !rt.SessionContextEnabled() || !rt.SessionContextEveryTurn() || rt.SessionContextRecentCount() != 12 {
		t.Fatalf("set did not stick: enabled=%v every=%v recent=%d",
			rt.SessionContextEnabled(), rt.SessionContextEveryTurn(), rt.SessionContextRecentCount())
	}

	// The list_sessions tool is gated on the per-runtime toggle.
	ctx := context.Background()
	agentRow, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "SC", Provider: "anthropic", MCPEnabled: true})
	if !rt.buildRegistry(ctx, agentRow).Has("list_sessions") {
		t.Error("list_sessions must be offered when session context is enabled")
	}
	rt.SetSessionContext(false, false, 0)
	if rt.buildRegistry(ctx, agentRow).Has("list_sessions") {
		t.Error("list_sessions must be absent when session context is disabled")
	}
}
