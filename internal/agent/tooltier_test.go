package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
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
	if !hasTool(full, "Read") || !hasTool(full, "WebFetch") {
		t.Fatalf("full catalog missing built-ins: %v", full)
	}

	if err := rt.db.SetWorkspaceToolConfig(ctx, db.WorkspaceToolConfig{DisabledTools: []string{"WebFetch"}}); err != nil {
		t.Fatalf("set workspace tool config: %v", err)
	}
	active := rt.ActiveToolCatalog(ctx)
	if hasTool(active, "WebFetch") {
		t.Fatal("workspace-disabled tool must be absent from the active catalog")
	}
	if !hasTool(active, "Read") {
		t.Fatal("non-disabled tool must remain active")
	}
	// The full catalog is unaffected by the denylist.
	if !hasTool(rt.WorkspaceToolCatalog(ctx), "WebFetch") {
		t.Fatal("full workspace catalog must still list disabled tools")
	}
}

// TestLazyCatalogSummarisesManyMCPTools verifies the load-on-demand catalog block
// lists built-in lazy tools in full but, past lazyCatalogMCPListLimit MCP tools,
// summarises them per server (count + tool_search pointer) instead of enumerating.
func TestLazyCatalogSummarisesManyMCPTools(t *testing.T) {
	// Few MCP tools → listed individually.
	small := []providers.ToolDef{
		{Name: "create_agent", Description: "self-mgmt"},
		{Name: "srvA__alpha", Description: "mcp tool alpha"},
		{Name: "srvA__beta", Description: "mcp tool beta"},
	}
	out := renderLazyToolCatalog(small)
	if !strings.Contains(out, "create_agent") || !strings.Contains(out, "srvA__alpha") {
		t.Fatalf("small catalog should list every tool, got:\n%s", out)
	}

	// Many MCP tools (> limit) → summarised per server, individuals dropped.
	big := []providers.ToolDef{{Name: "create_agent", Description: "self-mgmt"}}
	for i := 0; i < lazyCatalogMCPListLimit+5; i++ {
		big = append(big, providers.ToolDef{
			Name:        fmt.Sprintf("bigsrv__tool%d", i),
			Description: "an mcp tool",
		})
	}
	out = renderLazyToolCatalog(big)
	if !strings.Contains(out, "create_agent") {
		t.Error("built-in lazy tool must still be listed in full")
	}
	if strings.Contains(out, "bigsrv__tool0") {
		t.Error("individual MCP tools must NOT be enumerated past the limit")
	}
	if !strings.Contains(out, "tool_search") {
		t.Error("summary must point the model at tool_search")
	}
	if !strings.Contains(out, "bigsrv") {
		t.Error("summary must name the server")
	}
}

// TestReadOnlyAgentDemotesWriteTools verifies a read-only agent ships the read
// tools eagerly but the mutating tools (Write/Edit) are demoted to the
// load-on-demand catalog, while an auto agent keeps them eager.
func TestReadOnlyAgentDemotesWriteTools(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	auto := db.Agent{ID: "auto", MCPEnabled: true, PermissionMode: "auto"}
	if !hasTool(rt.ShippedToolCatalog(ctx, auto), "Write") {
		t.Fatal("auto agent must ship Write eagerly")
	}

	ro := db.Agent{ID: "ro", MCPEnabled: true, PermissionMode: "read-only"}
	if hasTool(rt.ShippedToolCatalog(ctx, ro), "Write") {
		t.Fatal("read-only agent must NOT ship Write eagerly")
	}
	if hasTool(rt.ShippedToolCatalog(ctx, ro), "Edit") {
		t.Fatal("read-only agent must NOT ship Edit eagerly")
	}
	if !hasTool(rt.LazyToolCatalog(ctx, ro), "Write") {
		t.Fatal("read-only agent must list Write as load-on-demand")
	}
	// Read tools stay eager regardless of permission mode.
	if !hasTool(rt.ShippedToolCatalog(ctx, ro), "Read") {
		t.Fatal("read-only agent must still ship Read eagerly")
	}
}

// TestAgentTierIntersectsWorkspace verifies an agent's effective tools are the
// intersection of the workspace-active set and the agent's allowlist, and that
// a workspace-disabled tool is removed even if the agent allowlists it.
func TestAgentTierIntersectsWorkspace(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	// Disable WebFetch workspace-wide.
	if err := rt.db.SetWorkspaceToolConfig(ctx, db.WorkspaceToolConfig{DisabledTools: []string{"WebFetch"}}); err != nil {
		t.Fatalf("set workspace tool config: %v", err)
	}

	// Agent allowlists both a disabled tool and an active one.
	allow, _ := json.Marshal([]string{"WebFetch", "memory_recall"})
	agent := db.Agent{ID: "a1", MCPEnabled: true, AllowedTools: string(allow)}

	eff := rt.ToolCatalog(ctx, agent)
	if hasTool(eff, "WebFetch") {
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
