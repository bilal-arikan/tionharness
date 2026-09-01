package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// seedGateServers registers the two servers the SES948 regression is about.
func seedGateServers(t *testing.T, rt *Runtime, ctx context.Context) {
	t.Helper()
	for _, name := range []string{"playwright", "codebase-memory-mcp"} {
		if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{
			Name: name, Transport: db.MCPTransportStdio, Command: "bunx", Enabled: true,
		}); err != nil {
			t.Fatalf("create %s server: %v", name, err)
		}
	}
}

func readCLIConfig(t *testing.T, path string) cliMCPConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg cliMCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return cfg
}

// SES948: a coder agent restricted to built-ins drove Playwright and the
// codebase-memory server for an entire codex-cli turn, because every ENABLED
// server was mounted regardless of the agent's allowlist. Neither server may
// reach the CLI process now.
func TestCLIMCPConfigWithholdsServersOutsideAgentAllowlist(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	seedGateServers(t, rt, ctx)

	ag := db.Agent{ID: "AGT102", MCPEnabled: true,
		AllowedTools: `["Read","LS","Glob","Grep","Write","Edit","Bash"]`}

	path, allowed, _, cleanup, err := rt.writeCLIMCPConfig(ctx, true, ag, tools.InteractionEndpoint{}, "auto")
	if err != nil {
		t.Fatalf("writeCLIMCPConfig: %v", err)
	}
	if path != "" {
		defer cleanup()
		if cfg := readCLIConfig(t, path); len(cfg.MCPServers) != 0 {
			t.Errorf("servers mounted despite a built-ins-only allowlist: %v", cfg.MCPServers)
		}
	}
	for _, a := range allowed {
		if a == "mcp__playwright" || a == "mcp__codebase-memory-mcp" {
			t.Errorf("withheld server still allowlisted: %q", a)
		}
	}

	spec, err := rt.codexMCPSpec(ctx, true, ag, tools.InteractionEndpoint{})
	if err != nil {
		t.Fatalf("codexMCPSpec: %v", err)
	}
	if len(spec.Servers) != 0 {
		t.Errorf("codex mounted servers outside the allowlist: %v", spec.Servers)
	}
}

// An allowlist that names one tool of a server keeps that server mounted — the
// gate is about "may not touch this server at all", not per-tool precision.
func TestCLIMCPConfigKeepsServerNamedByAllowlist(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	seedGateServers(t, rt, ctx)

	ag := db.Agent{ID: "AGT1", MCPEnabled: true,
		AllowedTools: `["Read","playwright__browser_click"]`}

	path, _, _, cleanup, err := rt.writeCLIMCPConfig(ctx, true, ag, tools.InteractionEndpoint{}, "auto")
	if err != nil {
		t.Fatalf("writeCLIMCPConfig: %v", err)
	}
	if path == "" {
		t.Fatal("no config written; playwright should have been mounted")
	}
	defer cleanup()
	cfg := readCLIConfig(t, path)
	if _, ok := cfg.MCPServers["playwright"]; !ok {
		t.Errorf("playwright missing though the allowlist names one of its tools: %v", cfg.MCPServers)
	}
	if _, ok := cfg.MCPServers["codebase-memory-mcp"]; ok {
		t.Errorf("unrelated server mounted: %v", cfg.MCPServers)
	}
}

// A blocked pattern covering the whole server unmounts it; blocking a single
// tool does not, since the server's other tools stay legitimate.
func TestCLIMCPConfigBlockedPatterns(t *testing.T) {
	cases := []struct {
		name      string
		overrides string
		wantPlay  bool
	}{
		{"server wildcard", `{"playwright__*":"blocked"}`, false},
		{"bare prefix wildcard", `{"playwright*":"blocked"}`, false},
		{"bare key", `{"playwright":"blocked"}`, false},
		{"single tool", `{"playwright__browser_click":"blocked"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
			ctx := context.Background()
			seedGateServers(t, rt, ctx)

			ag := db.Agent{ID: "AGT1", MCPEnabled: true, ToolOverrides: tc.overrides}
			path, _, _, cleanup, err := rt.writeCLIMCPConfig(ctx, true, ag, tools.InteractionEndpoint{}, "auto")
			if err != nil {
				t.Fatalf("writeCLIMCPConfig: %v", err)
			}
			if path == "" {
				t.Fatal("no config written; codebase-memory-mcp should still be mounted")
			}
			defer cleanup()
			cfg := readCLIConfig(t, path)
			if _, ok := cfg.MCPServers["playwright"]; ok != tc.wantPlay {
				t.Errorf("playwright mounted = %v, want %v (%v)", ok, tc.wantPlay, cfg.MCPServers)
			}
			if _, ok := cfg.MCPServers["codebase-memory-mcp"]; !ok {
				t.Errorf("unrelated server was dropped: %v", cfg.MCPServers)
			}
		})
	}
}

// An unconstrained agent (the zero value, and the common "no restrictions" case)
// mounts everything, exactly as before the gate existed.
func TestCLIMCPConfigUnconstrainedAgentMountsEverything(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	seedGateServers(t, rt, ctx)

	path, _, _, cleanup, err := rt.writeCLIMCPConfig(ctx, true, db.Agent{ID: "AGT1", MCPEnabled: true}, tools.InteractionEndpoint{}, "auto")
	if err != nil {
		t.Fatalf("writeCLIMCPConfig: %v", err)
	}
	if path == "" {
		t.Fatal("no config written")
	}
	defer cleanup()
	if cfg := readCLIConfig(t, path); len(cfg.MCPServers) != 2 {
		t.Errorf("mounted %d servers, want 2: %v", len(cfg.MCPServers), cfg.MCPServers)
	}
}

// group: keys classify BUILT-INS. A group-only allowlist must not be read as
// "targets some MCP server", and a group-only denylist must not unmount one.
func TestMCPServerGateIgnoresGroupKeys(t *testing.T) {
	deny, err := mcpServerGate(db.Agent{ToolOverrides: `{"group:files":"blocked"}`}, "")
	if err != nil {
		t.Fatalf("well-formed denylist: %v", err)
	}
	if deny == nil || !deny("playwright") {
		t.Error("a built-in group denylist unmounted an MCP server")
	}
	allow, err := mcpServerGate(db.Agent{AllowedTools: `["group:files"]`}, "")
	if err != nil {
		t.Fatalf("well-formed allowlist: %v", err)
	}
	if allow == nil || allow("playwright") {
		t.Error("a built-in group allowlist was read as targeting an MCP server")
	}
}

// A permission document that cannot be parsed must FAIL CLOSED: the error is
// surfaced and the gate mounts nothing. Swallowing it left both lists empty,
// which reads as "unconstrained" and mounted every MCP server into the CLI.
func TestMCPServerGateFailsClosedOnMalformedJSON(t *testing.T) {
	for _, ag := range []db.Agent{
		{ID: "A1", AllowedTools: `["playwright__`},
		{ID: "A2", ToolOverrides: `{"playwright*":`},
		{ID: "A3", BlockedTools: `["Bash"`},
		{ID: "A4", ToolOverrides: `{"Bash":"blokced"}`},
	} {
		gate, err := mcpServerGate(ag, "")
		if err == nil {
			t.Fatalf("agent %s: malformed permission JSON accepted", ag.ID)
		}
		if gate == nil || gate("playwright") {
			t.Fatalf("agent %s: gate must mount nothing after a parse failure", ag.ID)
		}
	}
}

func TestMalformedPermissionsFailClosedAcrossNativeAndCLIPaths(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	ag := db.Agent{ID: "AGT1", MCPEnabled: true, AllowedTools: `["Read"`}

	filter := rt.toolFilter(ctx, ag)
	if filter == nil || filter("Read") {
		t.Fatal("native tool filter allowed a tool after permission parsing failed")
	}
	if _, _, _, _, err := rt.writeCLIMCPConfig(ctx, true, ag, tools.InteractionEndpoint{}, "auto"); err == nil {
		t.Fatal("Claude CLI MCP setup accepted malformed permissions")
	}
	if _, err := rt.codexMCPSpec(ctx, true, ag, tools.InteractionEndpoint{}); err == nil {
		t.Fatal("Codex CLI MCP setup accepted malformed permissions")
	}
}

// The built-in worker profiles are a CONTRACT: their allowlists name built-ins
// only. Decided 2026-08-21 after SES948 showed CLI workers silently driving
// Playwright and codebase-memory. Widening a profile is a policy change — this
// test makes it a deliberate one (and forces the doc comment in subagent.go to be
// revisited with it).
//
// Note the empty exemptServer below: this asserts what the ALLOWLIST reaches, not
// what the agent can ultimately call. codebase-memory is exempt from the allowlist
// (allowlistExemptServer), so a profile worker DOES reach the code graph — see
// TestCodebaseMemoryIsExemptFromAllowlist. Everything else still needs a pattern.
func TestProfileAllowlistsReachOnlyValidatorUnityMCP(t *testing.T) {
	for id, prof := range defaultSubagentProfiles {
		gate, err := mcpServerGate(db.Agent{AllowedTools: mustJSON(t, prof.AllowedTools)}, "")
		if err != nil {
			t.Errorf("profile %q allowlist did not parse: %v", id, err)
			continue
		}
		if gate == nil {
			t.Errorf("profile %q has an empty allowlist — it constrains nothing", id)
			continue
		}
		for _, server := range []string{"playwright", "codebase-memory-mcp", "tionharness_extended", "unity-mcp"} {
			want := id == "validator" && server == "unity-mcp"
			if got := gate(server); got != want {
				t.Errorf("profile %q reaches MCP server %q; widening a profile is a policy change — update subagent.go's doc comment and _Docs/52 with it", id, server)
			}
		}
	}
}

func TestValidatorUnityMCPGateAcrossNativeClaudeAndCodex(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	for _, name := range []string{"unity-mcp", "playwright"} {
		if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{Name: name, Transport: db.MCPTransportStdio, Command: "tool", Enabled: true}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	ag := db.Agent{ID: "validator", MCPEnabled: true, AllowedTools: mustJSON(t, defaultSubagentProfiles["validator"].AllowedTools)}
	filter := rt.toolFilter(ctx, ag)
	if !filter("unity-mcp__read_console") || filter("playwright__browser_click") || filter("Write") || filter("Edit") {
		t.Fatal("native validator gate did not isolate Unity MCP validation access")
	}
	path, _, _, cleanup, err := rt.writeCLIMCPConfig(ctx, true, ag, tools.InteractionEndpoint{}, "auto")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	cfg := readCLIConfig(t, path)
	if _, ok := cfg.MCPServers["unity-mcp"]; !ok {
		t.Fatal("claude CLI omitted unity-mcp")
	}
	if _, ok := cfg.MCPServers["playwright"]; ok {
		t.Fatal("claude CLI mounted unrelated MCP server")
	}
	spec, err := rt.codexMCPSpec(ctx, true, ag, tools.InteractionEndpoint{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := spec.Servers["unity-mcp"]; !ok {
		t.Fatal("codex CLI omitted unity-mcp")
	}
	if _, ok := spec.Servers["playwright"]; ok {
		t.Fatal("codex CLI mounted unrelated MCP server")
	}
}

func TestCodexValidatorFreshTurnMountsOnlyUnityMCP(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	for _, name := range []string{"unity-mcp", "playwright", "codebase-memory-mcp"} {
		if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{Name: name, Transport: db.MCPTransportStdio, Command: "tool", Enabled: true}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	caller := db.Agent{ID: "caller", Provider: "codex-cli", MCPEnabled: true}
	validator, ephemeral, err := rt.resolveSubagentTarget(ctx, caller, "validator")
	if err != nil || !ephemeral {
		t.Fatalf("resolve validator: ephemeral=%v err=%v", ephemeral, err)
	}
	// This mirrors autonomousInteraction after profile filtering: validator has
	// no bridged TionHarness built-ins, so both tier name lists are empty.
	inter := tools.InteractionEndpoint{URL: "http://127.0.0.1:9999", Token: "token"}
	spec, err := rt.codexMCPSpec(ctx, validator.MCPEnabled, validator, inter)
	if err != nil {
		t.Fatalf("codexMCPSpec: %v", err)
	}
	if len(spec.Servers) != 1 {
		t.Fatalf("fresh validator mounted %d servers, want only unity-mcp: %v", len(spec.Servers), spec.Servers)
	}
	if _, ok := spec.Servers["unity-mcp"]; !ok {
		t.Fatalf("unity-mcp missing: %v", spec.Servers)
	}
	for _, forbidden := range []string{"playwright", "codebase-memory-mcp", interactionCoreKey, interactionExtendedKey} {
		if _, ok := spec.Servers[forbidden]; ok {
			t.Fatalf("forbidden server %q mounted: %v", forbidden, spec.Servers)
		}
	}
	filter := rt.toolFilter(ctx, validator)
	for _, forbidden := range []string{"Write", "Edit", "apply_patch"} {
		if filter == nil || filter(forbidden) {
			t.Fatalf("filesystem mutation tool %q allowed", forbidden)
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// The codebase-memory server is exempt from the ALLOWLIST on both paths: it is
// how an agent reads the repository, the same role Read/Glob/Grep play, and the
// static prompt orders every agent to prefer it over grep. Decided 2026-08-21.
func TestCodebaseMemoryIsExemptFromAllowlist(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.codebaseMemoryEnabled.Store(true)
	ctx := context.Background()
	if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{
		Name: "codebase-memory-mcp", Transport: db.MCPTransportStdio,
		Command: "C:/progs/codebase-memory-mcp/codebase-memory-mcp.exe", Enabled: true,
	}); err != nil {
		t.Fatalf("create cbm server: %v", err)
	}
	if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{
		Name: "playwright", Transport: db.MCPTransportStdio, Command: "bunx", Enabled: true,
	}); err != nil {
		t.Fatalf("create playwright server: %v", err)
	}

	// The exact SES948 agent: built-ins only.
	ag := db.Agent{ID: "AGT102", MCPEnabled: true,
		AllowedTools: `["Read","LS","Glob","Grep","Write","Edit","Bash"]`}

	if got := rt.allowlistExemptServer(ctx); got != "codebase-memory-mcp" {
		t.Fatalf("allowlistExemptServer = %q, want the configured cbm server name", got)
	}

	// Native path: the tool filter lets the graph through but still blocks playwright.
	filter := rt.toolFilter(ctx, ag)
	if filter == nil {
		t.Fatal("toolFilter is nil for a restricted agent")
	}
	if !filter("codebase-memory-mcp__search_code") {
		t.Error("native path still withholds the code graph from a built-ins-only agent")
	}
	if filter("playwright__browser_click") {
		t.Error("the exemption leaked to an unrelated MCP server")
	}

	// CLI path: the server is mounted; playwright is not.
	path, _, _, cleanup, err := rt.writeCLIMCPConfig(ctx, true, ag, tools.InteractionEndpoint{}, "auto")
	if err != nil {
		t.Fatalf("writeCLIMCPConfig: %v", err)
	}
	if path == "" {
		t.Fatal("no config written; the exempt server should have been mounted")
	}
	defer cleanup()
	cfg := readCLIConfig(t, path)
	if _, ok := cfg.MCPServers["codebase-memory-mcp"]; !ok {
		t.Errorf("exempt server not mounted for the CLI: %v", cfg.MCPServers)
	}
	if _, ok := cfg.MCPServers["playwright"]; ok {
		t.Errorf("the exemption leaked to playwright: %v", cfg.MCPServers)
	}
}

// The exemption covers the ALLOWLIST only. An operator who explicitly blocks the
// graph for one agent must still get that, on both paths — otherwise the switch
// in the UI would be a decoration.
func TestCodebaseMemoryExemptionYieldsToExplicitDenylist(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.codebaseMemoryEnabled.Store(true)
	ctx := context.Background()
	if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{
		Name: "codebase-memory-mcp", Transport: db.MCPTransportStdio,
		Command: "C:/progs/codebase-memory-mcp/codebase-memory-mcp.exe", Enabled: true,
	}); err != nil {
		t.Fatalf("create cbm server: %v", err)
	}

	ag := db.Agent{ID: "AGT1", MCPEnabled: true,
		ToolOverrides: `{"codebase-memory-mcp__*":"blocked"}`}

	if filter := rt.toolFilter(ctx, ag); filter == nil || filter("codebase-memory-mcp__search_code") {
		t.Error("an explicit denylist did not survive the exemption on the native path")
	}
	path, _, _, cleanup, err := rt.writeCLIMCPConfig(ctx, true, ag, tools.InteractionEndpoint{}, "auto")
	if err != nil {
		t.Fatalf("writeCLIMCPConfig: %v", err)
	}
	if path != "" {
		defer cleanup()
		if cfg := readCLIConfig(t, path); len(cfg.MCPServers) != 0 {
			t.Errorf("explicitly blocked server still mounted: %v", cfg.MCPServers)
		}
	}

	// And the prompt block must go quiet rather than order the agent to prefer
	// tools it cannot call (the SES948 contradiction).
	if codebaseMemoryCapability.Detect(ctx, rt, ag) {
		t.Error("capability block advertised to an agent that may not call the graph")
	}
}
