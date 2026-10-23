package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestUnknownSubagentTargetListsAgents: naming a deleted or misspelled agent
// returns the workspace's runnable agents, so the caller can correct the call
// instead of guessing a profile that may lack the needed tools.
func TestUnknownSubagentTargetListsAgents(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Board Keeper"}); err != nil {
		t.Fatal(err)
	}
	_, _, err := rt.resolveSubagentTarget(ctx, db.Agent{ID: "caller"}, "FootballOracle Validator Codex")
	if err == nil {
		t.Fatal("unknown target must fail")
	}
	for _, want := range []string{"validator", "Board Keeper"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}
