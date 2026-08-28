package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
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
	path, allowed, _, cleanup, err := rt.writeCLIMCPConfig(context.Background(), false, db.Agent{}, inter, "ask")
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

	core, ok := cfg.MCPServers["tionharness_interaction"]
	if !ok {
		t.Fatalf("core server entry missing:\n%s", data)
	}
	if !core.AlwaysLoad {
		t.Errorf("core server must set alwaysLoad:true:\n%s", data)
	}
	if !strings.HasSuffix(core.URL, "/core") {
		t.Errorf("core URL must target the /core tier, got %q", core.URL)
	}

	ext, ok := cfg.MCPServers["tionharness_extended"]
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
		"mcp__tionharness_interaction__Bash":              true,
		"mcp__tionharness_interaction__ask_user":          true,
		"mcp__tionharness_interaction__permission_prompt": true,
		"mcp__tionharness_extended":                       true, // wildcard covers update_session, create_agent, and any list_changed additions
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
		if strings.HasPrefix(a, "mcp__tionharness_extended__") {
			t.Errorf("extended tier should use a server-level wildcard, not per-tool entry %q", a)
		}
	}
}

func TestWriteCLIMCPConfigNativeWebSearchWithoutServers(t *testing.T) {
	has := func(list []string, name string) bool {
		for _, item := range list {
			if item == name {
				return true
			}
		}
		return false
	}

	for _, tc := range []struct {
		name            string
		nativeWebSearch bool
		wantDisallowed  bool
	}{
		{name: "disabled", wantDisallowed: true},
		{name: "enabled", nativeWebSearch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
			path, _, disallowed, cleanup, err := rt.writeCLIMCPConfig(
				context.Background(), false, db.Agent{NativeWebSearch: tc.nativeWebSearch}, tools.InteractionEndpoint{}, "ask",
			)
			if err != nil {
				t.Fatalf("writeCLIMCPConfig: %v", err)
			}
			defer cleanup()
			if path != "" {
				t.Fatalf("path = %q, want empty without MCP servers", path)
			}
			for _, tool := range []string{"WebSearch", "WebFetch"} {
				if got := has(disallowed, tool); got != tc.wantDisallowed {
					t.Errorf("disallowed contains %q = %v, want %v; list=%v", tool, got, tc.wantDisallowed, disallowed)
				}
			}
		})
	}
}

// TestWriteCLIMCPConfigRequiredCoreInvariant locks the WS17 invariant: a native CLI
// tool whose bridged replacement is prompt-/catalog-mandated (todo_write, use_skill)
// is suppressed ONLY while that bridge is actually advertised for the turn. When the
// bridge is filtered out of the advertised set, the native fallback must be KEPT so
// the model is never left with a mandated-but-missing tool (the "No such tool
// available" dead end). AskUserQuestion/ScheduleWakeup have no valid native fallback
// in one-shot -p mode, so they are suppressed unconditionally either way.
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
	var cfg cliMCPConfig
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

func TestWriteCLIMCPConfigRequiredCoreInvariant(t *testing.T) {
	has := func(list []string, name string) bool {
		for _, s := range list {
			if s == name {
				return true
			}
		}
		return false
	}

	// Case A: the bridges ARE advertised → their native shadows are suppressed.
	t.Run("bridge advertised suppresses native", func(t *testing.T) {
		rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
		inter := tools.InteractionEndpoint{
			URL:           "http://127.0.0.1:8090/mcp/interaction",
			Token:         "tok-A",
			CoreToolNames: []string{"todo_write", "use_skill", "ask_user"},
		}
		_, _, disallowed, cleanup, err := rt.writeCLIMCPConfig(context.Background(), false, db.Agent{}, inter, "ask")
		if err != nil {
			t.Fatalf("writeCLIMCPConfig: %v", err)
		}
		defer cleanup()
		for _, native := range []string{"TodoWrite", "TaskCreate", "Skill", "AskUserQuestion", "ScheduleWakeup"} {
			if !has(disallowed, native) {
				t.Errorf("expected native %q suppressed when its bridge is advertised; got %v", native, disallowed)
			}
		}
	})

	// Case B: the bridges are ABSENT (filtered out) → the native fallbacks are KEPT,
	// but the fallback-less natives stay suppressed.
	t.Run("bridge absent keeps native fallback", func(t *testing.T) {
		rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
		inter := tools.InteractionEndpoint{
			URL:           "http://127.0.0.1:8090/mcp/interaction",
			Token:         "tok-B",
			CoreToolNames: []string{"ask_user", "permission_prompt"}, // no todo_write / use_skill
		}
		_, _, disallowed, cleanup, err := rt.writeCLIMCPConfig(context.Background(), false, db.Agent{}, inter, "ask")
		if err != nil {
			t.Fatalf("writeCLIMCPConfig: %v", err)
		}
		defer cleanup()
		for _, native := range []string{"TodoWrite", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "Skill"} {
			if has(disallowed, native) {
				t.Errorf("native %q must NOT be suppressed when its bridge is not advertised (dead-end guard); got %v", native, disallowed)
			}
		}
		// No valid native fallback → suppressed regardless of the missing bridge.
		for _, native := range []string{"AskUserQuestion", "ScheduleWakeup"} {
			if !has(disallowed, native) {
				t.Errorf("fallback-less native %q must stay suppressed; got %v", native, disallowed)
			}
		}
	})
}
