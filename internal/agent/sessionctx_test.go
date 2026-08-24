package agent

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestRuntimeSessionContext verifies the always-on list_sessions tool is offered
// through buildRegistry (cross-session awareness is no longer configurable).
func TestRuntimeSessionContext(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())

	// The list_sessions tool is always offered (cross-session awareness is on).
	ctx := context.Background()
	agentRow, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "SC", Provider: "anthropic", MCPEnabled: true})
	if !rt.buildRegistry(ctx, agentRow).Has("list_sessions") {
		t.Error("list_sessions must always be offered")
	}
}
