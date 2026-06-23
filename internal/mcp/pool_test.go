package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestMain doubles as a fake stdio MCP server when SWARMGO_MCP_TEST_SERVER=1, so
// pool tests can exercise a real subprocess + JSON-RPC handshake without any
// external dependency. Otherwise it runs the package tests normally.
func TestMain(m *testing.M) {
	if os.Getenv("SWARMGO_MCP_TEST_SERVER") == "1" {
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
		Env:       map[string]string{"SWARMGO_MCP_TEST_SERVER": "1"},
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
