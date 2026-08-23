package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMain doubles as a fake stdio MCP server when TIONSWARM_MCP_TEST_SERVER=1, so
// pool tests can exercise a real subprocess + JSON-RPC handshake without any
// external dependency. Otherwise it runs the package tests normally.
func TestMain(m *testing.M) {
	if os.Getenv("TIONSWARM_MCP_TEST_SERVER") == "1" {
		runFakeServer()
		return
	}
	os.Exit(m.Run())
}

// runFakeServer speaks just enough MCP: initialize, tools/list, tools/call. The
// "activate" tool flips the advertised tool set from 1 to 2 tools and emits a
// tools/list_changed notification — mirroring the gateway's activate_tools.
func runFakeServer() {
	in := bufio.NewReader(os.Stdin)
	out := os.Stdout
	activated := false
	writeJSON := func(v any) {
		b, _ := json.Marshal(v)
		out.Write(append(b, '\n'))
	}
	toolList := func() []map[string]any {
		tools := []map[string]any{{"name": "echo", "description": "echo", "inputSchema": map[string]any{"type": "object"}}}
		if activated {
			tools = append(tools, map[string]any{"name": "extra", "description": "extra", "inputSchema": map[string]any{"type": "object"}})
		}
		return tools
	}
	for {
		line, err := in.ReadBytes('\n')
		if len(line) > 0 {
			var msg rpcMessage
			if json.Unmarshal(line, &msg) == nil && msg.Method != "" {
				switch msg.Method {
				case "initialize":
					writeJSON(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"protocolVersion": protocolVersion}})
				case "notifications/initialized":
					// no reply
				case "tools/list":
					writeJSON(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"tools": toolList()}})
				case "tools/call":
					// Decode the requested tool name.
					var p struct {
						Params struct {
							Name string `json:"name"`
						} `json:"params"`
					}
					_ = json.Unmarshal(line, &p)
					if p.Params.Name == "activate" {
						activated = true
						// Announce the change, then answer the call.
						writeJSON(map[string]any{"jsonrpc": "2.0", "method": "notifications/tools/list_changed"})
					}
					writeJSON(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{
						"content": []map[string]any{{"type": "text", "text": "ok:" + p.Params.Name}},
					}})
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func fakeCfg() ServerConfig {
	return ServerConfig{
		Name:      "fake",
		Transport: MCPTransportStdio,
		Command:   os.Args[0],
		Env:       map[string]string{"TIONSWARM_MCP_TEST_SERVER": "1"},
	}
}

func TestPoolReusesConnectionAndRefreshesOnListChanged(t *testing.T) {
	ctx := context.Background()
	p := NewPool()
	defer p.Close()
	cfg := fakeCfg()
	cfgs := []ServerConfig{cfg}

	// First catalog build: dials, lists → 1 tool.
	entries, cfgByServer, errs := p.Catalog(ctx, cfgs)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 tool, got %d", len(entries))
	}

	// Grab the live client; a second build must reuse the SAME connection.
	e := p.entry("fake")
	c1 := e.client
	if _, _, errs := p.Catalog(ctx, cfgs); len(errs) != 0 {
		t.Fatalf("second build errs: %v", errs)
	}
	if e.client != c1 {
		t.Fatal("expected the pool to reuse the live connection, not re-dial")
	}

	// Call the "activate" tool: the server flips to 2 tools and emits
	// tools/list_changed, which must invalidate the cached catalog.
	res, err := p.Call(ctx, cfgByServer, NamespaceTool("fake", "activate"), nil)
	if err != nil {
		t.Fatalf("activate call: %v", err)
	}
	if res.Text != "ok:activate" {
		t.Fatalf("activate result = %q", res.Text)
	}

	// The list_changed callback is async; poll until the refreshed catalog shows
	// the newly activated tool (still on the same persistent connection).
	deadline := time.Now().Add(3 * time.Second)
	var got int
	for time.Now().Before(deadline) {
		entries, _, _ = p.Catalog(ctx, cfgs)
		got = len(entries)
		if got == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got != 2 {
		t.Fatalf("after list_changed want 2 tools, got %d", got)
	}
	if e.client != c1 {
		t.Fatal("refresh must reuse the same connection, not re-dial")
	}
}

func TestHTTPPoolFreshCatalogSeesToolActivatedOnSameSession(t *testing.T) {
	const sessionID = "dynamic-session"
	var mu sync.Mutex
	activated := false
	initializeCount := 0
	wrongSession := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Method == "initialize" {
			mu.Lock()
			initializeCount++
			mu.Unlock()
			w.Header().Set("Mcp-Session-Id", sessionID)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": *req.ID,
				"result": map[string]any{"protocolVersion": protocolVersion},
			})
			return
		}
		if r.Header.Get("Mcp-Session-Id") != sessionID {
			mu.Lock()
			wrongSession = true
			mu.Unlock()
		}
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		writeResult := func(result any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
		}
		switch req.Method {
		case "tools/list":
			mu.Lock()
			on := activated
			mu.Unlock()
			list := []map[string]any{}
			if on {
				list = append(list, map[string]any{
					"name": "dynamic_test", "description": "dynamic test tool",
					"inputSchema": map[string]any{"type": "object"},
				})
			}
			writeResult(map[string]any{"tools": list})
		case "tools/call":
			if req.Params.Name == "activate" {
				mu.Lock()
				activated = true
				mu.Unlock()
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/tools/list_changed\"}\n\n")
				fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"activated\"}]}}\n\n", *req.ID)
				return
			}
			writeResult(map[string]any{"content": []map[string]any{{"type": "text", "text": "called:" + req.Params.Name}}})
		default:
			writeResult(map[string]any{})
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p := NewPool()
	defer p.Close()
	cfg := ServerConfig{Name: "dynamic-http", Transport: MCPTransportHTTP, URL: srv.URL}

	entries, cfgByServer, errs := p.Catalog(ctx, []ServerConfig{cfg})
	if len(errs) != 0 || len(entries) != 0 {
		t.Fatalf("initial catalog = %v, errs = %v; want no tools and no errors", entries, errs)
	}
	e := p.entry("dynamic-http")
	client := e.client
	if _, err := p.Call(ctx, cfgByServer, NamespaceTool(cfg.Name, "activate"), nil); err != nil {
		t.Fatalf("activate: %v", err)
	}

	entries, freshConfig, errs := p.Catalog(ctx, []ServerConfig{cfg})
	if len(errs) != 0 || len(entries) != 1 || entries[0].NamespacedName != "dynamic-http__dynamic_test" {
		t.Fatalf("fresh catalog = %v, errs = %v; want dynamic-http__dynamic_test", entries, errs)
	}
	result, err := p.Call(ctx, freshConfig, entries[0].NamespacedName, nil)
	if err != nil || result.Text != "called:dynamic_test" {
		t.Fatalf("fresh catalog call = %+v, err = %v", result, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if e.client != client || initializeCount != 1 || wrongSession {
		t.Fatalf("connection/session changed: sameClient=%v initializeCount=%d wrongSession=%v", e.client == client, initializeCount, wrongSession)
	}
}

func TestPoolReDialsOnConfigChange(t *testing.T) {
	ctx := context.Background()
	p := NewPool()
	defer p.Close()

	cfg := fakeCfg()
	if _, _, errs := p.Catalog(ctx, []ServerConfig{cfg}); len(errs) != 0 {
		t.Fatalf("build errs: %v", errs)
	}
	e := p.entry("fake")
	c1 := e.client

	// Change the config (an extra arg) → fingerprint changes → re-dial.
	cfg2 := cfg
	cfg2.Args = []string{"--variant", strconv.Itoa(1)}
	if _, _, errs := p.Catalog(ctx, []ServerConfig{cfg2}); len(errs) != 0 {
		t.Fatalf("rebuild errs: %v", errs)
	}
	if e.client == c1 {
		t.Fatal("expected a re-dial after config change")
	}
}

func TestPoolCallErrors(t *testing.T) {
	p := NewPool()
	defer p.Close()
	if _, err := p.Call(context.Background(), map[string]ServerConfig{}, "notnamespaced", nil); err == nil {
		t.Fatal("want error for non-namespaced name")
	}
	if _, err := p.Call(context.Background(), map[string]ServerConfig{}, NamespaceTool("ghost", "x"), nil); err == nil {
		t.Fatal("want error for unknown server")
	}
	// A guessed namespace must surface a close-match hint, not a bare failure.
	cfgByServer := map[string]ServerConfig{"codebase-memory-mcp": {Name: "codebase-memory-mcp"}}
	_, err := p.Call(context.Background(), cfgByServer, NamespaceTool("codebase_memory", "search_code"), nil)
	if err == nil {
		t.Fatal("want error for unknown (guessed) server")
	}
	if !strings.Contains(err.Error(), "did you mean codebase-memory-mcp") {
		t.Fatalf("expected suggestion in error, got %q", err.Error())
	}
}

func TestPoolCloseTerminatesConnections(t *testing.T) {
	ctx := context.Background()
	p := NewPool()
	cfg := fakeCfg()
	if _, _, errs := p.Catalog(ctx, []ServerConfig{cfg}); len(errs) != 0 {
		t.Fatalf("build errs: %v", errs)
	}
	c := p.entry("fake").client
	if c == nil || !c.Alive() {
		t.Fatal("expected a live client")
	}
	p.Close()
	if c.Alive() {
		t.Fatal("Close must terminate the connection")
	}
}

func TestConfigFingerprintChanges(t *testing.T) {
	a := ServerConfig{Name: "x", Command: "c", Args: []string{"1"}, Env: map[string]string{"A": "1"}}
	b := a
	if configFingerprint(a) != configFingerprint(b) {
		t.Fatal("identical configs must share a fingerprint")
	}
	b.Args = []string{"2"}
	if configFingerprint(a) == configFingerprint(b) {
		t.Fatal("arg change must change the fingerprint")
	}
}
