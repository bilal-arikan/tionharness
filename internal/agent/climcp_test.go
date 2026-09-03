package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/climcp"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestWriteCLIMCPConfigKeepsCodebaseMemoryEnv is the regression shield for the
// opposite of what this test once asserted: codebase-memory-mcp 0.10 allows only
// ONE cache root per OS account, so a per-workspace CBM_CACHE_DIR injected here
// made every concurrent CBM client (other workspaces, the CLI, external agents)
// fail with a cache-root conflict. The writer must pass the server's env through
// verbatim and let the server own its store.
func TestWriteCLIMCPConfigKeepsCodebaseMemoryEnv(t *testing.T) {
	container := t.TempDir()
	rt, _ := newTestRuntime(t, filepath.Join(container, "workspace"))
	rt.codebaseMemoryEnabled.Store(true)

	ctx := context.Background()
	cbmCmd := filepath.Join(container, "progs", "codebase-memory-mcp", "codebase-memory-mcp.exe")
	if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{
		Name: "codebase-memory-mcp", Transport: db.MCPTransportStdio, Command: cbmCmd, Enabled: true,
		EnvConfig: `{"CBM_CACHE_DIR":"D:/custom"}`,
	}); err != nil {
		t.Fatalf("create cbm server: %v", err)
	}
	if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{
		Name: "playwright", Transport: db.MCPTransportStdio, Command: "bunx", Enabled: true,
	}); err != nil {
		t.Fatalf("create other server: %v", err)
	}

	path, _, _, cleanup, err := rt.writeCLIMCPConfig(ctx, true, db.Agent{}, tools.InteractionEndpoint{}, "auto")
	if err != nil {
		t.Fatalf("writeCLIMCPConfig: %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg climcp.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}

	cbm, ok := cfg.MCPServers["codebase-memory-mcp"]
	if !ok {
		t.Fatalf("codebase-memory server missing from config: %v", cfg.MCPServers)
	}
	// Only what the operator configured travels — nothing is invented.
	if got := cbm.Env["CBM_CACHE_DIR"]; got != "D:/custom" {
		t.Errorf("CBM_CACHE_DIR = %q, want the operator-set value", got)
	}
	// Unrelated servers keep their own (empty) environment.
	if other := cfg.MCPServers["playwright"]; other.Env["CBM_CACHE_DIR"] != "" {
		t.Errorf("unrelated server was routed at a cbm store: %v", other.Env)
	}
}
