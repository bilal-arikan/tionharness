package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNamespaceTool(t *testing.T) {
	tests := []struct {
		name   string
		server string
		tool   string
		want   string
	}{
		{"bare tool", "server", "search", "server__search"},
		{"already namespaced", "server", "server__search", "server__search"},
		{"CLI namespaced", "server", "mcp__tionharness_interaction__activate_tools", "mcp__tionharness_interaction__activate_tools"},
		{"sanitized server", "server name", "server_name__search", "server_name__search"},
		{"different server", "server", "other__search", "server__other__search"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NamespaceTool(tt.server, tt.tool); got != tt.want {
				t.Fatalf("NamespaceTool(%q, %q) = %q, want %q", tt.server, tt.tool, got, tt.want)
			}
		})
	}
}

func TestBuildCatalogKeepsCLINamespaceIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		result := map[string]any{"protocolVersion": protocolVersion}
		if req.Method == "tools/list" {
			result = map[string]any{"tools": []map[string]any{{
				"name":        "mcp__tionharness_interaction__activate_tools",
				"description": "activate tools",
				"inputSchema": map[string]any{"type": "object"},
			}}}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	entries, errs := BuildCatalog(context.Background(), []ServerConfig{{
		Name:      "tionharness_interaction",
		Transport: MCPTransportHTTP,
		URL:       srv.URL,
	}})
	if len(errs) != 0 {
		t.Fatalf("BuildCatalog errors = %v", errs)
	}
	if len(entries) != 1 {
		t.Fatalf("BuildCatalog entries = %d, want 1", len(entries))
	}
	const want = "mcp__tionharness_interaction__activate_tools"
	if got := entries[0].NamespacedName; got != want {
		t.Fatalf("NamespacedName = %q, want %q", got, want)
	}
}
