package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
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

// TestWebSearchVisibleInWorkspaceCatalog verifies WebSearch is registered
// unconditionally (like WebFetch) so it appears in the workspace tools catalog —
// the catalog is built with an empty agent, and WebSearch must not be gated out
// of it. The claude-cli exclusion happens at the interaction-bridge layer, not by
// withholding registration, so visibility and CLI-suppression are independent.
func TestWebSearchVisibleInWorkspaceCatalog(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	full := rt.WorkspaceToolCatalog(ctx)
	if !hasTool(full, "WebSearch") {
		t.Fatalf("workspace catalog must list WebSearch (sibling of WebFetch): %v", full)
	}
	// And it must NOT be advertised to the claude-cli Interaction MCP bridge — the
	// CLI uses its own native WebSearch (mirrors WebFetch).
	if !cliLazyBridgeExcluded["WebSearch"] {
		t.Fatal("WebSearch must be in cliLazyBridgeExcluded so the CLI uses its native one")
	}
}

// TestLazyCatalogBundleLine verifies the one-line bundle hint: emitted on the
// native path when bundles exist, silent otherwise (so a workspace without
// bundles renders the byte-identical block it did before), and never on the CLI
// path, whose gateway resolves bundle membership from its own candidate set.
func TestLazyCatalogBundleLine(t *testing.T) {
	lazy := []providers.ToolDef{{Name: "create_agent", Description: "self-mgmt"}}
	counts := map[string]int{"group:agents": 3, "mcp:srv": 21}

	out := renderLazyToolCatalog(lazy, 0, "anthropic", nil, counts)
	if !strings.Contains(out, "Bundles: group:agents (3), mcp:srv (21)") {
		t.Fatalf("bundle line missing or unsorted:\n%s", out)
	}
	if !strings.Contains(out, "activate_tools(\"<bundle key>\")") {
		t.Fatalf("bundle line must say how to open one:\n%s", out)
	}

	if got := renderLazyToolCatalog(lazy, 0, "anthropic", nil, nil); strings.Contains(got, "Bundles:") {
		t.Fatalf("no bundles → no line:\n%s", got)
	}
	if got := renderLazyToolCatalog(lazy, 0, "claude-cli", nil, counts); strings.Contains(got, "Bundles:") {
		t.Fatalf("CLI form must not advertise native bundle counts:\n%s", got)
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
	out := renderLazyToolCatalog(small, 0, "anthropic", nil, nil)
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
	// A curated server description must ride the per-server summary line so the
	// model keeps a semantic hint even when individual tools are dropped.
	out = renderLazyToolCatalog(big, 0, "anthropic", map[string]string{"bigsrv": "graph code search; prefer over grep"}, nil)
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
	if !strings.Contains(out, "graph code search; prefer over grep") {
		t.Errorf("per-server summary must carry the server description, got:\n%s", out)
	}
}

// TestLazyCatalogHidesSelfManageBehindSkillPointer verifies the rendered block
// does NOT enumerate hidden self-management tools but emits a single pointer to
// the tionharness-self-management skill (and still names the visible lazy tools).
func TestLazyCatalogHidesSelfManageBehindSkillPointer(t *testing.T) {
	visible := []providers.ToolDef{{Name: "WebFetch", Description: "fetch a page"}}
	out := renderLazyToolCatalog(visible, 12, "anthropic", nil, nil)
	if !strings.Contains(out, "WebFetch") {
		t.Error("visible lazy tools must still be listed")
	}
	if !strings.Contains(out, "tionharness-self-management") {
		t.Error("hidden suite must be replaced by a pointer to the self-management skill")
	}
	if !strings.Contains(out, "12 self-management tools") {
		t.Errorf("pointer must state the hidden count, got:\n%s", out)
	}
	// With no hidden tools, no pointer line.
	out = renderLazyToolCatalog(visible, 0, "anthropic", nil, nil)
	if strings.Contains(out, "tionharness-self-management") {
		t.Error("no pointer when there are no hidden tools")
	}
	// Empty + no hidden → empty block.
	if renderLazyToolCatalog(nil, 0, "anthropic", nil, nil) != "" {
		t.Error("empty catalog with no hidden tools must render nothing")
	}
}

// TestLazyCatalogCLIFormNamespacesNames verifies the claude-cli rendering of the
// load-on-demand catalog: lazy built-in tools carry the EXTENDED Interaction MCP
// prefix (the deferred tier), MCP tools carry the mcp__ prefix, CLI-native built-ins
// (WebFetch) are dropped, and the guidance loads deferred built-ins via the gateway
// activate_tools (Doc 52) — external MCP tools still noted for ToolSearch.
func TestLazyCatalogCLIFormNamespacesNames(t *testing.T) {
	lazy := []providers.ToolDef{
		{Name: "update_session"},                  // lazy built-in → extended namespace
		{Name: "WebFetch", Description: "fetch"},  // CLI-native → dropped
		{Name: "srvA__alpha", Description: "mcp"}, // MCP → mcp__ prefix
	}
	out := renderLazyToolCatalog(lazy, 3, "claude-cli", nil, nil)

	if !strings.Contains(out, "mcp__tionharness_extended__update_session") {
		t.Errorf("CLI form must namespace lazy built-ins under the extended tier:\n%s", out)
	}
	if !strings.Contains(out, "mcp__srvA__alpha") {
		t.Errorf("CLI form must prefix MCP tools with mcp__:\n%s", out)
	}
	if strings.Contains(out, "WebFetch") {
		t.Errorf("CLI-native WebFetch must be dropped from the CLI catalog:\n%s", out)
	}
	if !strings.Contains(out, "activate_tools") {
		t.Errorf("CLI form must load deferred built-ins via activate_tools (gateway):\n%s", out)
	}
	// ToolSearch select: is for the EXTERNAL MCP entries only — it must appear in
	// their separator line, and only after the deferred built-ins are listed.
	sel := strings.Index(out, "select:")
	if sel < 0 || !strings.Contains(out[sel-200:sel], "EXTERNAL MCP tools") {
		t.Errorf("CLI form must mark the MCP entries as ToolSearch select:\n%s", out)
	}
	if sel < strings.Index(out, "mcp__tionharness_extended__update_session") {
		t.Errorf("CLI form must NOT instruct ToolSearch select for built-ins (gateway path):\n%s", out)
	}
	// Self-management pointer uses the namespaced use_skill on the CLI path.
	if !strings.Contains(out, "mcp__tionharness_interaction__use_skill") {
		t.Errorf("CLI self-management pointer must namespace use_skill:\n%s", out)
	}

	// Native form keeps bare names + activate_tools (regression guard).
	nat := renderLazyToolCatalog(lazy, 0, "anthropic", nil, nil)
	if !strings.Contains(nat, "- `update_session`") || !strings.Contains(nat, "- `WebFetch`") {
		t.Errorf("native form keeps bare names incl. WebFetch:\n%s", nat)
	}
	if !strings.Contains(nat, "activate_tools") {
		t.Errorf("native form keeps activate_tools guidance:\n%s", nat)
	}
}

// TestLazyCatalogSummaryFormSpellsToolSearchSyntax guards the SUMMARISED branch
// (MCP tools above lazyCatalogMCPListLimit): the CLI form must spell out the
// ToolSearch query syntax (`select:<name>`) in both the server summary and the
// self-management pointer, otherwise agents call ToolSearch with an invented
// parameter and get an InputValidationError. Native form keeps tool_search.
func TestLazyCatalogSummaryFormSpellsToolSearchSyntax(t *testing.T) {
	var lazy []providers.ToolDef
	for i := 0; i <= lazyCatalogMCPListLimit; i++ { // one past the list limit → summary branch
		lazy = append(lazy, providers.ToolDef{Name: fmt.Sprintf("srvA__tool%d", i), Description: "mcp"})
	}

	out := renderLazyToolCatalog(lazy, 7, "claude-cli", nil, nil)
	if strings.Contains(out, "srvA__tool0") {
		t.Fatalf("fixture must exceed the list limit and summarise instead:\n%s", out)
	}
	if strings.Count(out, "select:<name>") != 2 {
		t.Errorf("CLI summary + self-management pointer must both spell the ToolSearch select: syntax:\n%s", out)
	}

	nat := renderLazyToolCatalog(lazy, 7, "anthropic", nil, nil)
	if strings.Contains(nat, "select:") {
		t.Errorf("native form uses tool_search, not ToolSearch select:\n%s", nat)
	}
	if !strings.Contains(nat, "`tool_search(\"keyword\")`") {
		t.Errorf("native summary must keep the tool_search hint:\n%s", nat)
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
	allow, _ := json.Marshal([]string{"WebFetch", "WebSearch"})
	agent := db.Agent{ID: "a1", MCPEnabled: true, AllowedTools: string(allow)}

	eff := rt.ToolCatalog(ctx, agent)
	if hasTool(eff, "WebFetch") {
		t.Fatal("workspace-disabled tool must not reach the agent even if allowlisted")
	}
	if !hasTool(eff, "WebSearch") {
		t.Fatal("allowlisted + active tool must be offered to the agent")
	}
	// A tool that is active but not in the allowlist is excluded.
	if hasTool(eff, "todo_write") {
		t.Fatal("tool outside the agent allowlist must be excluded")
	}
}
