package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// hasTool reports whether a tool name is present in a catalog.
func hasTool(defs []providers.ToolDef, name string) bool {
	for _, d := range defs {
		if d.Name == name {
			return true
		}
	}
	return false
}

// TestWorkspaceTierFiltersCatalog verifies the workspace-level denylist removes
// tools from the active catalog while the full catalog still lists everything.
func TestWorkspaceTierFiltersCatalog(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	full := rt.WorkspaceToolCatalog(ctx)
	if !hasTool(full, "get_current_time") || !hasTool(full, "http_get") {
		t.Fatalf("full catalog missing built-ins: %v", full)
	}

	if err := rt.db.SetWorkspaceToolConfig(ctx, db.WorkspaceToolConfig{DisabledTools: []string{"http_get"}}); err != nil {
		t.Fatalf("set workspace tool config: %v", err)
	}
	active := rt.ActiveToolCatalog(ctx)
	if hasTool(active, "http_get") {
		t.Fatal("workspace-disabled tool must be absent from the active catalog")
	}
	if !hasTool(active, "get_current_time") {
		t.Fatal("non-disabled tool must remain active")
	}
	// The full catalog is unaffected by the denylist.
	if !hasTool(rt.WorkspaceToolCatalog(ctx), "http_get") {
		t.Fatal("full workspace catalog must still list disabled tools")
	}
}

// TestAgentTierIntersectsWorkspace verifies an agent's effective tools are the
// intersection of the workspace-active set and the agent's allowlist, and that
// a workspace-disabled tool is removed even if the agent allowlists it.
func TestAgentTierIntersectsWorkspace(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	// Disable get_current_time workspace-wide.
	if err := rt.db.SetWorkspaceToolConfig(ctx, db.WorkspaceToolConfig{DisabledTools: []string{"get_current_time"}}); err != nil {
		t.Fatalf("set workspace tool config: %v", err)
	}

	// Agent allowlists both a disabled tool and an active one.
	allow, _ := json.Marshal([]string{"get_current_time", "memory_recall"})
	agent := db.Agent{ID: "a1", MCPEnabled: true, AllowedTools: string(allow)}

	eff := rt.ToolCatalog(ctx, agent)
	if hasTool(eff, "get_current_time") {
		t.Fatal("workspace-disabled tool must not reach the agent even if allowlisted")
	}
	if !hasTool(eff, "memory_recall") {
		t.Fatal("allowlisted + active tool must be offered to the agent")
	}
	// A tool that is active but not in the allowlist is excluded.
	if hasTool(eff, "todo_write") {
		t.Fatal("tool outside the agent allowlist must be excluded")
	}
}
