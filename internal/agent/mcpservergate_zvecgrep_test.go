package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// zvec-grep joined codebase-memory as allowlist-exempt infrastructure on
// 2026-09-14: it is how an agent finds repository material by intent, and its
// prompt block steers every agent to it. Same two paths, same limits.
func TestZvecGrepIsExemptFromAllowlist(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateMCPServer(ctx, zvecGrepRow()); err != nil {
		t.Fatalf("create zvec-grep server: %v", err)
	}
	if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{
		Name: "playwright", Transport: db.MCPTransportStdio, Command: "bunx", Enabled: true,
	}); err != nil {
		t.Fatalf("create playwright server: %v", err)
	}
	ag := db.Agent{ID: "AGT102", MCPEnabled: true,
		AllowedTools: `["Read","LS","Glob","Grep","Write","Edit","Bash"]`}

	filter := rt.toolFilter(ctx, ag)
	if filter == nil {
		t.Fatal("toolFilter is nil for a restricted agent")
	}
	if !filter("zvec_grep__zvec_grep_search") {
		t.Error("native path withholds zvec-grep from a built-ins-only agent")
	}
	if filter("playwright__browser_click") {
		t.Error("the exemption leaked to an unrelated MCP server")
	}

	path, _, _, cleanup, err := rt.writeCLIMCPConfig(ctx, true, ag, tools.InteractionEndpoint{}, "auto")
	if err != nil {
		t.Fatalf("writeCLIMCPConfig: %v", err)
	}
	if path == "" {
		t.Fatal("no config written; the exempt server should have been mounted")
	}
	defer cleanup()
	cfg := readCLIConfig(t, path)
	if _, ok := cfg.MCPServers["zvec_grep"]; !ok {
		t.Errorf("exempt server not mounted for the CLI: %v", cfg.MCPServers)
	}
	if _, ok := cfg.MCPServers["playwright"]; ok {
		t.Errorf("the exemption leaked to playwright: %v", cfg.MCPServers)
	}

	// The workspace switch takes the exemption away with the rest of the feature.
	rt.SetZvecGrep(false)
	if keys := rt.allowlistExemptServers(ctx); len(keys) != 0 {
		t.Errorf("allowlistExemptServers = %q with the switch off", keys)
	}
	if f := rt.toolFilter(ctx, ag); f == nil || f("zvec_grep__zvec_grep_search") {
		t.Error("switched-off zvec-grep still bypasses the allowlist")
	}
}

// Both infrastructure servers at once: each is exempt, neither shadows the other.
func TestAllowlistExemptServersListsBoth(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	for _, row := range []db.MCPServer{
		{
			Name: "codebase-memory-mcp", Transport: db.MCPTransportStdio, Enabled: true,
			Command: "C:/progs/codebase-memory-mcp/codebase-memory-mcp.exe",
		},
		zvecGrepRow(),
	} {
		if _, err := rt.db.CreateMCPServer(ctx, row); err != nil {
			t.Fatalf("create %s: %v", row.Name, err)
		}
	}
	keys := rt.allowlistExemptServers(ctx)
	if len(keys) != 2 || keys[0] != "codebase-memory-mcp" || keys[1] != "zvec_grep" {
		t.Fatalf("allowlistExemptServers = %q, want [codebase-memory-mcp zvec_grep]", keys)
	}
	gate, err := mcpServerGate(db.Agent{AllowedTools: `["Read"]`}, keys...)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	for _, key := range keys {
		if !gate(key) {
			t.Errorf("CLI gate withholds exempt server %q", key)
		}
	}
	if gate("playwright") {
		t.Error("CLI gate mounted a non-exempt server for a built-ins-only agent")
	}
}
