package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// TestMCPServerWarmsRuntimePool proves the connection-test endpoint uses the
// workspace-owned pool while preserving its existing JSON response contract.
func TestHTTPMCPTestWarmsLiveCatalog(t *testing.T) {
	var initializes atomic.Int32
	var lists atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode MCP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			initializes.Add(1)
			result = map[string]any{"protocolVersion": "2025-03-26"}
		case "tools/list":
			lists.Add(1)
			result = map[string]any{"tools": []map[string]any{{
				"name": "echo", "description": "echo tool",
				"inputSchema": map[string]any{"type": "object"},
			}}}
		default:
			t.Errorf("unexpected MCP method %q", req.Method)
			result = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}))
	defer backend.Close()

	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	server, err := database.CreateMCPServer(context.Background(), db.MCPServer{
		Name: "fake", Transport: db.MCPTransportHTTP, URL: backend.URL, Args: "[]",
		EnvConfig: "{}", HeadersConfig: "{}", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create MCP server: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runtime := agent.NewRuntime(database, providers.NewRegistry(), agent.NewTunables(), t.TempDir(), t.TempDir(), nil, nil, "WS1", "Test", nil, logger)
	defer runtime.CloseMCP()
	wsp := &workspace.Workspace{DB: database, Runtime: runtime}
	s := &Server{logger: logger}

	req := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/"+server.ID+"/test", nil)
	req.SetPathValue("id", server.ID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	recorder := httptest.NewRecorder()
	s.handleTestMCPServer(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		OK        bool `json:"ok"`
		ToolCount int  `json:"toolCount"`
		Tools     []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode API response: %v", err)
	}
	if !response.OK || response.ToolCount != 1 || len(response.Tools) != 1 || response.Tools[0].Name != "echo" {
		t.Fatalf("unexpected API response: %+v", response)
	}
	if initializes.Load() != 1 || lists.Load() != 1 {
		t.Fatalf("endpoint calls: initialize=%d tools/list=%d, want 1 each", initializes.Load(), lists.Load())
	}

	entries, _, errs := runtime.MCPPool().Catalog(context.Background(), []mcp.ServerConfig{mcpServerConfig(server)})
	if len(errs) != 0 || len(entries) != 1 || entries[0].Tool.Name != "echo" {
		t.Fatalf("cached catalog: entries=%+v errors=%v", entries, errs)
	}
	if initializes.Load() != 1 || lists.Load() != 1 {
		t.Fatalf("cached catalog opened another connection: initialize=%d tools/list=%d", initializes.Load(), lists.Load())
	}
}

// TestMCPSignatureDedup verifies the importable-server identity: same wiring (name
// + transport + command + args + url) yields the same signature regardless of id
// or enabled state, while any wiring difference yields a distinct one. This is what
// dedupes a server shared across workspaces and hides ones already present here.
func TestMCPSignatureDedup(t *testing.T) {
	base := db.MCPServer{
		ID: "MCP1", Name: "codebase-memory-mcp", Transport: "stdio",
		Command: "cbm.exe", Args: "[]", URL: "", Enabled: true,
	}
	// Same wiring, different id + disabled → same signature.
	twin := base
	twin.ID = "MCP99"
	twin.Enabled = false
	if mcpSignature(base) != mcpSignature(twin) {
		t.Fatalf("identical wiring must share a signature:\n%q\n%q", mcpSignature(base), mcpSignature(twin))
	}
	// Name compared case-insensitively (display casing must not fork the identity).
	upper := base
	upper.Name = "Codebase-Memory-MCP"
	if mcpSignature(base) != mcpSignature(upper) {
		t.Fatalf("name casing must not change the signature")
	}
	// Any wiring change → distinct signature.
	for _, diff := range []func(m *db.MCPServer){
		func(m *db.MCPServer) { m.Command = "other.exe" },
		func(m *db.MCPServer) { m.Args = `["-y"]` },
		func(m *db.MCPServer) { m.Transport = "http" },
		func(m *db.MCPServer) { m.URL = "https://x" },
		func(m *db.MCPServer) { m.Name = "different" },
	} {
		m := base
		diff(&m)
		if mcpSignature(m) == mcpSignature(base) {
			t.Fatalf("a wiring change must produce a distinct signature, got equal for %+v", m)
		}
	}
}
