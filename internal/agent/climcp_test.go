package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// TestWriteCLIMCPConfigTwoTierInteraction locks the claude-cli 2.1.x+ two-tier
// bridge: the Interaction MCP endpoint is emitted as two server entries — a core
// server marked alwaysLoad (eager, never deferred) and an extended server (deferred
// via the CLI's ToolSearch) — with each tier's tools allowlisted under its own
// namespace.
func TestWriteCLIMCPConfigTwoTierInteraction(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	inter := tools.InteractionEndpoint{
		URL:               "http://127.0.0.1:8090/mcp/interaction",
		Token:             "tok-123",
		CoreToolNames:     []string{"Bash", "ask_user", "permission_prompt"},
		ExtendedToolNames: []string{"update_session", "create_agent"},
	}

	// mcpEnabled=false so the test doesn't depend on any stored MCP servers.
	path, allowed, _, cleanup, err := rt.writeCLIMCPConfig(context.Background(), false, inter, "ask")
	if err != nil {
		t.Fatalf("writeCLIMCPConfig: %v", err)
	}
	defer cleanup()
	if path == "" {
		t.Fatal("expected a config path for a populated interaction endpoint")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Type       string `json:"type"`
			URL        string `json:"url"`
			AlwaysLoad bool   `json:"alwaysLoad"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v\n%s", err, data)
	}

	core, ok := cfg.MCPServers["tionswarm_interaction"]
	if !ok {
		t.Fatalf("core server entry missing:\n%s", data)
	}
	if !core.AlwaysLoad {
		t.Errorf("core server must set alwaysLoad:true:\n%s", data)
	}
	if !strings.HasSuffix(core.URL, "/core") {
		t.Errorf("core URL must target the /core tier, got %q", core.URL)
	}

	ext, ok := cfg.MCPServers["tionswarm_extended"]
	if !ok {
		t.Fatalf("extended server entry missing:\n%s", data)
	}
	if ext.AlwaysLoad {
		t.Errorf("extended server must NOT set alwaysLoad (it is deferred):\n%s", data)
	}
	if !strings.HasSuffix(ext.URL, "/extended") {
		t.Errorf("extended URL must target the /extended tier, got %q", ext.URL)
	}

	// Core tier is allowlisted per-tool under the core server key; the extended tier
	// uses a single SERVER-LEVEL wildcard (no per-tool suffix) so tools added later via
	// tools/list_changed are already permitted and the persistent-session fingerprint
	// stays stable (Doc 52 §3-D).
	want := map[string]bool{
		"mcp__tionswarm_interaction__Bash":              true,
		"mcp__tionswarm_interaction__ask_user":          true,
		"mcp__tionswarm_interaction__permission_prompt": true,
		"mcp__tionswarm_extended":                       true, // wildcard covers update_session, create_agent, and any list_changed additions
	}
	got := map[string]bool{}
	for _, a := range allowed {
		got[a] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("allowlist missing %q; got %v", w, allowed)
		}
	}
	// The extended tier must NOT be enumerated per-tool any more (that per-tool churn is
	// exactly what the wildcard replaces).
	for _, a := range allowed {
		if strings.HasPrefix(a, "mcp__tionswarm_extended__") {
			t.Errorf("extended tier should use a server-level wildcard, not per-tool entry %q", a)
		}
	}
}
