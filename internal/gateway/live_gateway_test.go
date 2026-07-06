package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/mcp"
)

// secretMCP is a fake backend MCP server exposing get_secret, which returns a marker so
// the live test can prove the full chain (claude -> gateway -> pool -> backend) worked.
type secretMCP struct{}

func (secretMCP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusOK)
		return
	}
	var req struct {
		ID     *int   `json:"id"`
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	write := func(result any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}
	switch req.Method {
	case "initialize":
		write(map[string]any{"protocolVersion": "2025-06-18"})
	case "tools/list":
		write(map[string]any{"tools": []map[string]any{
			{"name": "get_secret", "description": "Returns the secret token.", "inputSchema": map[string]any{"type": "object"}},
		}})
	case "tools/call":
		txt := "unknown"
		if req.Params.Name == "get_secret" {
			txt = "SECRET=GW_EXT_OK_88"
		}
		write(map[string]any{"content": []map[string]any{{"type": "text", "text": txt}}})
	default:
		write(map[string]any{})
	}
}

// TestLiveExternalGateway is a REAL end-to-end validation of Doc 52 Faz 3: a live
// claude-cli client connects to TionSwarm's external /mcp/gateway (the real gateway.Server
// over a real mcp.Pool) with a bearer token, and in ONE turn calls activate_tools -> the
// gateway connects the backend + pushes tools/list_changed -> claude re-lists -> calls the
// proxied backend tool -> reports the secret. Proves the full chain
// claude -> gateway -> pool -> backend MCP. Spends tokens; gated behind TIONSWARM_LIVE_CLI=1.
//
//	TIONSWARM_LIVE_CLI=1 go test ./internal/gateway/ -run TestLiveExternalGateway -v
func TestLiveExternalGateway(t *testing.T) {
	if os.Getenv("TIONSWARM_LIVE_CLI") != "1" {
		t.Skip("set TIONSWARM_LIVE_CLI=1 to run the live external-gateway validation")
	}
	bin := "claude"
	if p := os.Getenv("TIONSWARM_CLAUDE_BIN"); p != "" {
		bin = p
	}
	model := "claude-fable-5"
	if m := os.Getenv("TIONSWARM_CLAUDE_MODEL"); m != "" {
		model = m
	}

	// Real backend MCP + real pool + real gateway, token-authed.
	back := httptest.NewServer(secretMCP{})
	defer back.Close()
	pool := mcp.NewPool()
	defer pool.Close()
	servers := func(context.Context, string) ([]mcp.ServerConfig, error) {
		return []mcp.ServerConfig{{Name: "fake", Transport: "http", URL: back.URL}}, nil
	}
	const token = "gw-live-tok"
	b := NewBackend(func(string) *mcp.Pool { return pool }, servers, nil)
	srv := NewServer(b, func(tok string) bool { return tok == token }, nil)
	b.SetServer(srv)
	gw := httptest.NewServer(srv)
	defer gw.Close()

	cfg := map[string]any{"mcpServers": map[string]any{
		"gw": map[string]any{"type": "http", "url": gw.URL,
			"headers": map[string]string{"Authorization": "Bearer " + token}},
	}}
	cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
	cfgFile, err := os.CreateTemp(t.TempDir(), "gw-ext-*.json")
	if err != nil {
		t.Fatal(err)
	}
	cfgFile.Write(cfgBytes)
	cfgFile.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	prompt := "You are connected to an MCP gateway named gw. Do this in ONE turn: " +
		"(1) call activate_tools with servers=[\"fake\"]. (2) This registers a tool (its name contains " +
		"\"get_secret\") via tools/list_changed. (3) Call that tool. (4) Report the exact SECRET string it " +
		"returns. Do not ask me anything."
	args := []string{"-p", prompt,
		"--mcp-config", cfgFile.Name(), "--strict-mcp-config",
		"--allowedTools", "mcp__gw",
		"--model", model, "--output-format", "stream-json", "--verbose"}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "ENABLE_TOOL_SEARCH=auto")
	out, runErr := cmd.CombinedOutput()
	t.Logf("claude exit=%v\n--- tail ---\n%s", runErr, tail(string(out), 3500))

	const secret = "GW_EXT_OK_88"
	if !strings.Contains(string(out), secret) {
		t.Fatalf("external gateway chain failed: want %q in output (claude -> gateway -> pool -> backend)", secret)
	}
	fmt.Fprintln(os.Stdout, "LIVE EXTERNAL GATEWAY VALIDATION: secret obtained via /mcp/gateway ✓")
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
