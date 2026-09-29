package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

func TestWorkspaceToolsCachedReturnsBeforeMCPDial(t *testing.T) {
	var calls atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
		var rpc struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rpc); err != nil {
			t.Errorf("decode MCP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if rpc.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		calls.Add(1)
		var result any
		switch rpc.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-03-26"}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{
				"name": "echo", "description": "echo tool",
				"inputSchema": map[string]any{"type": "object"},
			}}}
		default:
			t.Errorf("unexpected MCP method %q", rpc.Method)
			result = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *rpc.ID, "result": result})
	}))
	defer backend.Close()

	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.CreateMCPServer(context.Background(), db.MCPServer{
		Name: "fake", Transport: db.MCPTransportHTTP, URL: backend.URL, Args: "[]",
		EnvConfig: "{}", HeadersConfig: "{}", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runtime := agent.NewRuntime(database, providers.NewRegistry(), agent.NewTunables(), t.TempDir(), t.TempDir(), nil, nil, "WS1", "Test", nil, logger)
	defer runtime.CloseMCP()
	s := &Server{logger: logger}
	wsp := &workspace.Workspace{DB: database, Runtime: runtime}

	list := func(path string) []string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
		recorder := httptest.NewRecorder()
		s.handleWorkspaceTools(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(response.Tools))
		for _, tool := range response.Tools {
			names = append(names, tool.Name)
		}
		return names
	}

	cached := list("/api/workspace-tools?cached=1")
	if !slices.Contains(cached, "Read") || slices.Contains(cached, "fake__echo") {
		t.Fatalf("cached tools = %v", cached)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("cached request dialed MCP server: %d calls", got)
	}

	refreshed := list("/api/workspace-tools")
	if !slices.Contains(refreshed, "fake__echo") {
		t.Fatalf("refreshed tools omit MCP tool: %v", refreshed)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("full request made %d MCP calls, want initialize and tools/list", got)
	}
}
