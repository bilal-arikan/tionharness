package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// An unknown "group:" key is a hard 400: silently dropping it would leave the
// user believing a ban is in force while nothing matches it.
func TestSetAgentToolsRejectsUnknownGroupKey(t *testing.T) {
	s := &Server{}
	body := `{"mcpEnabled":true,"toolOverrides":{"group:nope":"blocked"}}`
	r := httptest.NewRequest(http.MethodPost, "/api/agents/A1/tools", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSetAgentTools(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "group:nope") {
		t.Fatalf("error should name the offending key: %s", w.Body.String())
	}
}

// The groups payload lists every built-in category present in the catalog plus
// one "<ns>__*" row per MCP server, labelled with the configured server name.
func TestAgentToolGroupsPayloadShape(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()
	if _, err := database.CreateMCPServer(ctx, db.MCPServer{Name: "linear"}); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}

	wsp := &workspace.Workspace{DB: database}
	names := []string{"Read", "Bash", "WebSearch", "linear__issue", "linear__team"}
	groups, err := agentToolGroups(ctx, wsp, names)
	if err != nil {
		t.Fatalf("agentToolGroups: %v", err)
	}

	byKey := map[string]agentToolGroup{}
	for _, g := range groups {
		byKey[g.Key] = g
	}
	files, ok := byKey["group:files"]
	if !ok {
		t.Fatalf("group:files missing from %v", groups)
	}
	if files.Kind != "builtin" || files.Label != "files" || files.Count != 2 {
		t.Fatalf("unexpected files group: %+v", files)
	}
	if len(files.Tools) != 2 || files.Tools[0] != "Bash" || files.Tools[1] != "Read" {
		t.Fatalf("files group tools should be sorted members: %v", files.Tools)
	}
	if _, ok := byKey["group:artifacts"]; ok {
		t.Fatal("a category with no catalog member must not be offered")
	}
	mcpGroup, ok := byKey["linear__*"]
	if !ok {
		t.Fatalf("MCP server group missing from %v", groups)
	}
	if mcpGroup.Kind != "mcp" || mcpGroup.Label != "linear" || mcpGroup.Count != 2 {
		t.Fatalf("unexpected MCP group: %+v", mcpGroup)
	}
	// Built-in categories come first, MCP servers after.
	if groups[len(groups)-1].Kind != "mcp" {
		t.Fatalf("MCP groups must be last: %+v", groups)
	}
}
