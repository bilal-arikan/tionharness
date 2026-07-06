package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/mcp"
)

// fakeMCP is a minimal Streamable HTTP MCP backend exposing one tool, "echo".
type fakeMCP struct{}

func (fakeMCP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
			{"name": "echo", "description": "echo back", "inputSchema": map[string]any{"type": "object"}},
		}})
	case "tools/call":
		write(map[string]any{"content": []map[string]any{{"type": "text", "text": "ok:" + req.Params.Name}}})
	default:
		write(map[string]any{})
	}
}

// TestGatewayActivateAndProxy exercises the external gateway end to end over a real
// mcp.Pool: meta-tools only at first, activate_tools connects the backend and grows the
// surface, the proxied tool call round-trips, and deactivate shrinks it again.
func TestGatewayActivateAndProxy(t *testing.T) {
	back := httptest.NewServer(fakeMCP{})
	defer back.Close()

	pool := mcp.NewPool()
	defer pool.Close()
	servers := func(context.Context, string) ([]mcp.ServerConfig, error) {
		return []mcp.ServerConfig{{Name: "fake", Transport: "http", URL: back.URL}}, nil
	}
	b := NewBackend(func(string) *mcp.Pool { return pool }, servers, nil)
	srv := NewServer(b, nil, nil)
	b.SetServer(srv)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const sid = "s1"

	// Starts with the 4 meta-tools only.
	if got := len(b.Tools(sid)); got != 4 {
		t.Fatalf("session must start with 4 meta-tools, got %d: %v", got, toolNames(b.Tools(sid)))
	}

	// list_servers sees the backend as available.
	if res, _ := b.Call(ctx, sid, "list_servers", nil); !strings.Contains(res.Text, "fake") || !strings.Contains(res.Text, "available") {
		t.Fatalf("list_servers = %q", res.Text)
	}

	// Activate the backend server → its tools appear (namespaced).
	res, err := b.Call(ctx, sid, "activate_tools", json.RawMessage(`{"servers":["fake"]}`))
	if err != nil {
		t.Fatalf("activate err: %v", err)
	}
	if res.IsError || !strings.Contains(res.Text, "activated") {
		t.Fatalf("activate result = %q", res.Text)
	}
	echoName := mcp.NamespaceTool("fake", "echo")
	if !hasTool(b.Tools(sid), echoName) {
		t.Fatalf("echo must be advertised after activation as %q, got %v", echoName, toolNames(b.Tools(sid)))
	}

	// active_tools reports it.
	if res, _ := b.Call(ctx, sid, "active_tools", nil); !strings.Contains(res.Text, "fake") {
		t.Fatalf("active_tools = %q", res.Text)
	}

	// Proxied call round-trips through the pool to the backend.
	res, err = b.Call(ctx, sid, echoName, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("proxied call err: %v", err)
	}
	if res.IsError || !strings.Contains(res.Text, "ok:echo") {
		t.Fatalf("proxied echo = %q (isErr=%v)", res.Text, res.IsError)
	}

	// Deactivate shrinks the surface back to meta-tools only.
	if _, err := b.Call(ctx, sid, "deactivate_tools", json.RawMessage(`{"servers":["fake"]}`)); err != nil {
		t.Fatalf("deactivate err: %v", err)
	}
	if got := len(b.Tools(sid)); got != 4 {
		t.Fatalf("after deactivate expected 4 meta-tools, got %d: %v", got, toolNames(b.Tools(sid)))
	}
}

// TestGatewayAuth verifies the bearer auth gate and initialize session minting over
// the HTTP transport.
func TestGatewayAuth(t *testing.T) {
	pool := mcp.NewPool()
	defer pool.Close()
	b := NewBackend(func(string) *mcp.Pool { return pool }, func(context.Context, string) ([]mcp.ServerConfig, error) { return nil, nil }, nil)
	srv := NewServer(b, func(tok string) bool { return tok == "secret" }, nil)
	b.SetServer(srv)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	// No token → 401.
	if code := post(t, ts.URL, "", initBody); code != http.StatusUnauthorized {
		t.Fatalf("missing token: want 401, got %d", code)
	}
	// Wrong token → 401.
	if code := post(t, ts.URL, "Bearer nope", initBody); code != http.StatusUnauthorized {
		t.Fatalf("bad token: want 401, got %d", code)
	}
	// Correct token → 200 + a minted Mcp-Session-Id.
	req, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(initBody))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("good token: want 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Mcp-Session-Id") == "" {
		t.Fatal("initialize must mint an Mcp-Session-Id")
	}
}

func post(t *testing.T, url, auth, body string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// TestGatewayPerWorkspaceRouting verifies a session sees only its bound workspace's
// servers (X-Workspace-Id → OpenSession), so one gateway serves many workspaces.
func TestGatewayPerWorkspaceRouting(t *testing.T) {
	back := httptest.NewServer(fakeMCP{})
	defer back.Close()
	pool := mcp.NewPool()
	defer pool.Close()
	// Only workspace "wsA" exposes the server.
	servers := func(_ context.Context, wsID string) ([]mcp.ServerConfig, error) {
		if wsID == "wsA" {
			return []mcp.ServerConfig{{Name: "fake", Transport: "http", URL: back.URL}}, nil
		}
		return nil, nil
	}
	b := NewBackend(func(string) *mcp.Pool { return pool }, servers, nil)
	srv := NewServer(b, nil, nil)
	b.SetServer(srv)
	ctx := context.Background()

	b.OpenSession("sA", "wsA")
	if res, _ := b.Call(ctx, "sA", "list_servers", nil); !strings.Contains(res.Text, "fake") {
		t.Fatalf("wsA session must see its server, got %q", res.Text)
	}
	b.OpenSession("sB", "wsB")
	if res, _ := b.Call(ctx, "sB", "list_servers", nil); strings.Contains(res.Text, "fake") {
		t.Fatalf("wsB session must NOT see wsA's server, got %q", res.Text)
	}
}

// TestGatewayAuditRecordsProxiedCalls verifies the audit sink fires for proxied backend
// tool calls (server+tool+ok) but NOT for meta-tools (gateway-audit parity).
func TestGatewayAuditRecordsProxiedCalls(t *testing.T) {
	back := httptest.NewServer(fakeMCP{})
	defer back.Close()
	pool := mcp.NewPool()
	defer pool.Close()
	servers := func(context.Context, string) ([]mcp.ServerConfig, error) {
		return []mcp.ServerConfig{{Name: "fake", Transport: "http", URL: back.URL}}, nil
	}
	b := NewBackend(func(string) *mcp.Pool { return pool }, servers, nil)
	srv := NewServer(b, nil, nil)
	b.SetServer(srv)
	var mu sync.Mutex
	var entries []AuditEntry
	b.SetAudit(func(e AuditEntry) { mu.Lock(); entries = append(entries, e); mu.Unlock() })

	ctx := context.Background()
	// Meta-tool: not audited.
	b.Call(ctx, "s1", "activate_tools", json.RawMessage(`{"servers":["fake"]}`))
	if len(entries) != 0 {
		t.Fatalf("meta-tools must not be audited, got %+v", entries)
	}
	// Proxied backend tool: audited.
	b.Call(ctx, "s1", mcp.NamespaceTool("fake", "echo"), json.RawMessage(`{}`))
	if len(entries) != 1 || entries[0].Server != "fake" || entries[0].Tool != "echo" || !entries[0].OK {
		t.Fatalf("proxied call must be audited as fake/echo/ok, got %+v", entries)
	}
}

func toolNames(specs []ToolSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Name
	}
	return out
}
func hasTool(specs []ToolSpec, name string) bool {
	for _, s := range specs {
		if s.Name == name {
			return true
		}
	}
	return false
}
