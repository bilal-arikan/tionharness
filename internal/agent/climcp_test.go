package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/tools"
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
		ExtendedToolNames: []string{"set_session_goal", "create_agent"},
	}

	// mcpEnabled=false so the test doesn't depend on any stored MCP servers.
	path, allowed, _, cleanup, err := rt.writeCLIMCPConfig(context.Background(), false, inter)
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

	core, ok := cfg.MCPServers["swarmgo_interaction"]
	if !ok {
		t.Fatalf("core server entry missing:\n%s", data)
	}
	if !core.AlwaysLoad {
		t.Errorf("core server must set alwaysLoad:true:\n%s", data)
	}
	if !strings.HasSuffix(core.URL, "/core") {
		t.Errorf("core URL must target the /core tier, got %q", core.URL)
	}

	ext, ok := cfg.MCPServers["swarmgo_extended"]
	if !ok {
		t.Fatalf("extended server entry missing:\n%s", data)
	}
	if ext.AlwaysLoad {
		t.Errorf("extended server must NOT set alwaysLoad (it is deferred):\n%s", data)
	}
	if !strings.HasSuffix(ext.URL, "/extended") {
		t.Errorf("extended URL must target the /extended tier, got %q", ext.URL)
	}

	// Allowlist entries are namespaced under each tier's server key.
	want := map[string]bool{
		"mcp__swarmgo_interaction__Bash":              true,
		"mcp__swarmgo_interaction__ask_user":          true,
		"mcp__swarmgo_interaction__permission_prompt": true,
		"mcp__swarmgo_extended__set_session_goal":     true,
		"mcp__swarmgo_extended__create_agent":         true,
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
}
