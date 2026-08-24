package interaction

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// gwFakeBackend mimics the api gateway backend (Doc 52 Faz 1-b) against the REAL
// streaming interaction.Server: the core tier exposes activate_tools; the extended
// tier exposes gizmo_secret ONLY after it is activated. activate_tools marks it active
// and pushes tools/list_changed so the CLI re-lists and can call it the same turn.
type gwFakeBackend struct {
	token string
	srv   *Server

	mu          sync.Mutex
	active      bool
	sawActivate bool
	sawSecret   bool
}

func (b *gwFakeBackend) Valid(token string) bool { return token == b.token }

func (b *gwFakeBackend) Tools(_, tier string) []ToolSpec {
	switch tier {
	case "core":
		return []ToolSpec{{
			Name:        "activate_tools",
			Description: "Load an on-demand tool into this session so you can call it. Pass names in `tools`.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"tools":{"type":"array","items":{"type":"string"}}},"required":["tools"]}`),
		}}
	case "extended":
		b.mu.Lock()
		defer b.mu.Unlock()
		if !b.active {
			return nil // empty until activated — the gateway dynamic surface
		}
		return []ToolSpec{{
			Name:        "gizmo_secret",
			Description: "Returns the secret token.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		}}
	default:
		return nil
	}
}

func (b *gwFakeBackend) Call(_ context.Context, _, name string, args json.RawMessage) (CallResult, error) {
	switch name {
	case "activate_tools":
		b.mu.Lock()
		b.active = true
		b.sawActivate = true
		b.mu.Unlock()
		// Push list_changed so the CLI re-lists extended and gizmo_secret appears.
		b.srv.PushToolsChanged(b.token)
		return CallResult{Text: "activated: gizmo_secret — it is now callable"}, nil
	case "gizmo_secret":
		b.mu.Lock()
		active := b.active
		if active {
			b.sawSecret = true
		}
		b.mu.Unlock()
		if !active {
			return CallResult{Text: "gizmo_secret is locked; call activate_tools first", IsError: true}, nil
		}
		return CallResult{Text: "SECRET=GATEWAY_LIVE_OK_77"}, nil
	default:
		return CallResult{Text: "unknown tool: " + name, IsError: true}, nil
	}
}

// TestLiveGatewayActivate is a REAL end-to-end validation of Doc 52 Faz 1-b: a live
// claude-cli turn against the REAL streaming interaction.Server, wired exactly like
// writeCLIMCPConfig (core = alwaysLoad, extended = deferred, extended allowlisted with a
// SERVER-LEVEL wildcard). It proves the model can, in ONE turn: call activate_tools →
// the server pushes tools/list_changed on the extended connection → the CLI re-lists →
// calls the newly-appeared gizmo_secret (permitted by the wildcard) → reports the secret.
// Spends tokens; gated behind TIONHARNESS_LIVE_CLI=1.
//
//	TIONHARNESS_LIVE_CLI=1 go test ./internal/interaction/ -run TestLiveGatewayActivate -v
func TestLiveGatewayActivate(t *testing.T) {
	if os.Getenv("TIONHARNESS_LIVE_CLI") != "1" {
		t.Skip("set TIONHARNESS_LIVE_CLI=1 to run the live gateway activate validation")
	}
	bin := "claude"
	if p := os.Getenv("TIONHARNESS_CLAUDE_BIN"); p != "" {
		bin = p
	}
	model := "claude-fable-5"
	if m := os.Getenv("TIONHARNESS_CLAUDE_MODEL"); m != "" {
		model = m
	}

	const token = "live-tok-123"
	backend := &gwFakeBackend{token: token}
	srv := NewServer(backend, nil)
	backend.srv = srv

	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Mirror writeCLIMCPConfig: two entries at the SAME endpoint via /core and /extended
	// path suffixes; core alwaysLoad; extended deferred. Same Bearer on both.
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"gwcore": map[string]any{
				"type": "http", "url": ts.URL + "/core",
				"headers":    map[string]string{"Authorization": "Bearer " + token},
				"alwaysLoad": true,
			},
			"gwext": map[string]any{
				"type": "http", "url": ts.URL + "/extended",
				"headers": map[string]string{"Authorization": "Bearer " + token},
			},
		},
	}
	cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
	cfgFile, err := os.CreateTemp(t.TempDir(), "gw-mcp-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfgFile.Write(cfgBytes); err != nil {
		t.Fatal(err)
	}
	cfgFile.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	prompt := "You have a core tool `activate_tools`. Do this in ONE turn: " +
		"(1) call activate_tools with tools=[\"gizmo_secret\"]. " +
		"(2) A tool named gizmo_secret then becomes available (via tools/list_changed). " +
		"(3) Call gizmo_secret. (4) Report the exact SECRET string it returns. Do not ask me anything."

	args := []string{
		"-p", prompt,
		"--mcp-config", cfgFile.Name(), "--strict-mcp-config",
		"--allowedTools", "mcp__gwcore__activate_tools", "mcp__gwext",
		"--model", model,
		"--output-format", "stream-json", "--verbose",
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "ENABLE_TOOL_SEARCH=auto")
	out, runErr := cmd.CombinedOutput()
	t.Logf("claude exit err=%v\n--- output tail ---\n%s", runErr, tailStr(string(out), 4000))

	const secret = "GATEWAY_LIVE_OK_77"
	if !strings.Contains(string(out), secret) {
		t.Fatalf("model did not obtain the secret via activate→list_changed→call; want %q in output", secret)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if !backend.sawActivate {
		t.Errorf("backend never saw activate_tools")
	}
	if !backend.sawSecret {
		t.Errorf("backend never saw a successful gizmo_secret call (post-activation)")
	}
	fmt.Fprintln(os.Stdout, "LIVE GATEWAY VALIDATION: secret obtained in-turn ✓")
}

func tailStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
