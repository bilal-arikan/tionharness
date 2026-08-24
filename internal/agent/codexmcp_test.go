package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestCodexMCPSpecLeavesConfigPathEmpty locks the documented asymmetry with the
// claude path: codex takes its MCP servers as config.toml keys, so the spec must
// carry the Servers map and NEVER a config file path.
func TestCodexMCPSpecLeavesConfigPathEmpty(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	inter := tools.InteractionEndpoint{
		URL:           "http://127.0.0.1:8090/mcp/interaction",
		Token:         "tok-123",
		CoreToolNames: []string{"Bash", "ask_user"},
	}

	spec, err := rt.codexMCPSpec(context.Background(), false, db.Agent{}, inter)
	if err != nil {
		t.Fatalf("codexMCPSpec: %v", err)
	}
	if spec.ConfigPath != "" {
		t.Errorf("ConfigPath must stay empty on the codex path, got %q", spec.ConfigPath)
	}
	if len(spec.Servers) == 0 {
		t.Fatal("expected servers for a populated interaction endpoint")
	}
	// The claude-only fields have no codex meaning and must not be invented.
	if spec.PermissionPrompt != "" {
		t.Errorf("PermissionPrompt must stay empty (codex exec rejects approvals), got %q", spec.PermissionPrompt)
	}
	if spec.SettingsPath != "" {
		t.Errorf("SettingsPath must stay empty on the codex path, got %q", spec.SettingsPath)
	}
}

// TestCodexMCPSpecTwoTierInteraction locks that the Interaction bridge is mounted
// as the same two tiers the claude path uses, over Streamable HTTP with literal
// Authorization headers (which renderCodexConfig emits as http_headers).
func TestCodexMCPSpecTwoTierInteraction(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	inter := tools.InteractionEndpoint{
		// A trailing slash must not produce a doubled separator.
		URL:               "http://127.0.0.1:8090/mcp/interaction/",
		Token:             "tok-abc",
		CoreToolNames:     []string{"Bash", "ask_user", "todo_write"},
		ExtendedToolNames: []string{"update_session"},
	}

	spec, err := rt.codexMCPSpec(context.Background(), false, db.Agent{}, inter)
	if err != nil {
		t.Fatalf("codexMCPSpec: %v", err)
	}

	core, ok := spec.Servers[interactionCoreKey]
	if !ok {
		t.Fatalf("core server entry missing: %v", spec.Servers)
	}
	if core.URL != "http://127.0.0.1:8090/mcp/interaction/core?full=1" {
		t.Errorf("core URL = %q, want the /core tier (no doubled slash) with ?full=1 (codex never re-lists on tools/list_changed, so the gate must not be offered)", core.URL)
	}
	if core.Transport != db.MCPTransportHTTP {
		t.Errorf("core transport = %q, want %q", core.Transport, db.MCPTransportHTTP)
	}
	if got := core.Headers["Authorization"]; got != "Bearer tok-abc" {
		t.Errorf("core Authorization = %q, want the bearer token", got)
	}
	if core.Command != "" {
		t.Errorf("an HTTP tier must not carry a command, got %q", core.Command)
	}

	ext, ok := spec.Servers[interactionExtendedKey]
	if !ok {
		t.Fatalf("extended server entry missing: %v", spec.Servers)
	}
	if ext.URL != "http://127.0.0.1:8090/mcp/interaction/extended?full=1" {
		t.Errorf("extended URL = %q, want the /extended tier with ?full=1", ext.URL)
	}

	// Tool ids must stay identical to the claude path so mcp.SplitNamespaced and
	// the existing trace stripping keep working unchanged.
	wantAllowed := map[string]bool{
		"mcp__" + interactionCoreKey + "__Bash":     true,
		"mcp__" + interactionCoreKey + "__ask_user": true,
		"mcp__" + interactionExtendedKey:            true,
	}
	got := map[string]bool{}
	for _, a := range spec.AllowedTools {
		got[a] = true
	}
	for w := range wantAllowed {
		if !got[w] {
			t.Errorf("allowlist missing %q; got %v", w, spec.AllowedTools)
		}
	}
	// Carried through unchanged even though the codex renderer ignores them today.
	if len(spec.DisallowedTools) == 0 {
		t.Errorf("expected the codex native suppressions to be carried, got none")
	}
}

// TestCodexMCPSpecExternalServers locks that enabled external MCP servers reach
// the spec with the transport-specific fields renderCodexConfig consumes: URL +
// Headers for remote servers, Command/Args/Env for stdio ones.
func TestCodexMCPSpecExternalServers(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{
		Name: "playwright", Transport: db.MCPTransportStdio, Command: "bunx", Enabled: true,
		Args: `["@playwright/mcp"]`, EnvConfig: `{"PW_HEADLESS":"1"}`,
	}); err != nil {
		t.Fatalf("create stdio server: %v", err)
	}
	if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{
		Name: "remote-tools", Transport: db.MCPTransportHTTP, URL: "https://example.test/mcp",
		Enabled: true, HeadersConfig: `{"Authorization":"Bearer remote-tok"}`,
	}); err != nil {
		t.Fatalf("create http server: %v", err)
	}

	spec, err := rt.codexMCPSpec(ctx, true, db.Agent{}, tools.InteractionEndpoint{})
	if err != nil {
		t.Fatalf("codexMCPSpec: %v", err)
	}
	if spec.ConfigPath != "" {
		t.Errorf("ConfigPath must stay empty, got %q", spec.ConfigPath)
	}

	stdio, ok := spec.Servers["playwright"]
	if !ok {
		t.Fatalf("stdio server missing: %v", spec.Servers)
	}
	if stdio.Command != "bunx" || len(stdio.Args) != 1 || stdio.Args[0] != "@playwright/mcp" {
		t.Errorf("stdio command/args not carried: %+v", stdio)
	}
	if stdio.Env["PW_HEADLESS"] != "1" {
		t.Errorf("stdio env not carried verbatim: %+v", stdio.Env)
	}
	if stdio.URL != "" {
		t.Errorf("a stdio server must not carry a URL, got %q", stdio.URL)
	}

	remote, ok := spec.Servers["remote-tools"]
	if !ok {
		t.Fatalf("http server missing: %v", spec.Servers)
	}
	if remote.URL != "https://example.test/mcp" {
		t.Errorf("http URL = %q", remote.URL)
	}
	if remote.Headers["Authorization"] != "Bearer remote-tok" {
		t.Errorf("http headers not carried: %+v", remote.Headers)
	}
	if remote.Command != "" {
		t.Errorf("an http server must not carry a command, got %q", remote.Command)
	}

	// Server-level wildcards, same shape the claude path uses.
	for _, want := range []string{"mcp__playwright", "mcp__remote-tools"} {
		found := false
		for _, a := range spec.AllowedTools {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("allowlist missing %q; got %v", want, spec.AllowedTools)
		}
	}
}

// TestCodexMCPSpecEmptyWhenNothingToWire: no external servers and no interaction
// endpoint means no MCP delegation at all, signalled by a zero spec.
func TestCodexMCPSpecEmptyWhenNothingToWire(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	spec, err := rt.codexMCPSpec(context.Background(), false, db.Agent{}, tools.InteractionEndpoint{})
	if err != nil {
		t.Fatalf("codexMCPSpec: %v", err)
	}
	if len(spec.Servers) != 0 {
		t.Errorf("expected no servers, got %v", spec.Servers)
	}
	if len(spec.AllowedTools) != 0 || len(spec.DisallowedTools) != 0 {
		t.Errorf("expected empty tool lists, got allow=%v disallow=%v",
			spec.AllowedTools, spec.DisallowedTools)
	}
}

// TestCodexMCPSpecKeysMatchClaudePath is the cross-dialect invariant: for the
// same inputs both writers must produce the SAME server keys, so a tool id
// (mcp__<key>__<tool>) means the same thing on either CLI.
func TestCodexMCPSpecKeysMatchClaudePath(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{
		Name: "codebase-memory-mcp", Transport: db.MCPTransportStdio,
		Command: "codebase-memory-mcp.exe", Enabled: true,
	}); err != nil {
		t.Fatalf("create server: %v", err)
	}
	inter := tools.InteractionEndpoint{
		URL:           "http://127.0.0.1:8090/mcp/interaction",
		Token:         "tok-x",
		CoreToolNames: []string{"Bash"},
	}

	spec, err := rt.codexMCPSpec(ctx, true, db.Agent{}, inter)
	if err != nil {
		t.Fatalf("codexMCPSpec: %v", err)
	}
	claudeAllowed := func() []string {
		_, allowed, _, cleanup, err := rt.writeCLIMCPConfig(ctx, true, db.Agent{}, inter, "auto")
		if err != nil {
			t.Fatalf("writeCLIMCPConfig: %v", err)
		}
		defer cleanup()
		return allowed
	}()

	// Every server key the codex spec mounts must appear as a claude allowlist
	// prefix, i.e. the two writers agree on naming.
	for key := range spec.Servers {
		prefix := "mcp__" + key
		found := false
		for _, a := range claudeAllowed {
			if a == prefix || strings.HasPrefix(a, prefix+"__") {
				found = true
			}
		}
		if !found {
			t.Errorf("codex server key %q has no counterpart in the claude allowlist %v", key, claudeAllowed)
		}
	}
}
