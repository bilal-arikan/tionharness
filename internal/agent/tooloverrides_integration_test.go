package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// newOverrideRuntime builds a runtime + an agent carrying the given override
// document, for exercising the full precedence chain through buildRegistry.
func newOverrideRuntime(t *testing.T, overrides string) (*Runtime, db.Agent) {
	t.Helper()
	workDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rt, _ := newTestRuntime(t, workDir)
	ag, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name: "Ov", Provider: "anthropic", MCPEnabled: true, ToolOverrides: overrides,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return rt, ag
}

// The precedence chain is code default < workspace override < agent override.
// list_sessions is name-only by code default; the workspace pins it to hidden;
// the agent must still be able to pull it back to full.
func TestAgentOverrideBeatsWorkspaceOverride(t *testing.T) {
	ctx := context.Background()
	rt, ag := newOverrideRuntime(t, `{"list_sessions":"full"}`)
	if err := rt.db.SetWorkspaceToolConfig(ctx, db.WorkspaceToolConfig{
		ToolVisibility: map[string]string{"list_sessions": tools.VisibilityHidden},
	}); err != nil {
		t.Fatalf("set workspace tool config: %v", err)
	}

	// Without the agent override the workspace tier wins (baseline check).
	base := rt.buildRegistry(ctx, db.Agent{}).VisibilityOf("list_sessions")
	if base != tools.VisibilityHidden {
		t.Fatalf("workspace override should apply to a bare agent, got %q", base)
	}
	if got := rt.ToolVisibilityFunc(ctx, ag)("list_sessions"); got != tools.VisibilityFull {
		t.Fatalf("agent override must beat the workspace one, got %q", got)
	}
}

// The 'blocked' tier is not a visibility state: it must drop the tool from the
// agent's catalog entirely rather than reach the registry as a tier.
func TestBlockedTierRemovesToolFromCatalog(t *testing.T) {
	ctx := context.Background()
	rt, ag := newOverrideRuntime(t, `{"Read":"blocked","Write":"hidden"}`)

	for _, d := range rt.ToolCatalog(ctx, ag) {
		if d.Name == "Read" {
			t.Fatal("a blocked tool must not appear in the agent's catalog")
		}
	}
	// A sibling visibility override in the same map still applies normally.
	if got := rt.ToolVisibilityFunc(ctx, ag)("Write"); got != tools.VisibilityHidden {
		t.Fatalf("visibility override alongside a block got %q", got)
	}
	// And an untouched tool is still offered.
	if !rt.ToolAllowedFunc(ctx, ag)("Write") {
		t.Fatal("Write must still be allowed")
	}
	if rt.ToolAllowedFunc(ctx, ag)("Read") {
		t.Fatal("Read must be filtered out")
	}
}

// An agent stored under the OLD model (BlockedTools only, no override map) keeps
// its ban — the unification must not silently unblock anything.
func TestLegacyDenylistStillBlocks(t *testing.T) {
	ctx := context.Background()
	workDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rt, _ := newTestRuntime(t, workDir)
	ag, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Legacy", Provider: "anthropic", MCPEnabled: true, BlockedTools: `["Read"]`,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if rt.ToolAllowedFunc(ctx, ag)("Read") {
		t.Fatal("legacy denylist entry must still block")
	}
}

// UpdateAgentTools is the single writer: it stores the override map AND mirrors
// its blocked entries into BlockedTools so older readers stay correct.
func TestUpdateAgentToolsMirrorsBlockedTools(t *testing.T) {
	ctx := context.Background()
	rt, ag := newOverrideRuntime(t, "{}")
	if err := rt.db.UpdateAgentTools(ctx, ag.ID, true, `{"Read":"blocked","Write":"full","Grep":"blocked"}`); err != nil {
		t.Fatalf("update agent tools: %v", err)
	}
	stored, err := rt.db.GetAgent(ctx, ag.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if stored.BlockedTools != `["Grep","Read"]` {
		t.Fatalf("BlockedTools mirror = %s, want sorted [Grep Read]", stored.BlockedTools)
	}
	if got := ParseToolOverrides(stored)["Write"]; got != tools.VisibilityFull {
		t.Fatalf("non-blocked override lost on write: %q", got)
	}
}
