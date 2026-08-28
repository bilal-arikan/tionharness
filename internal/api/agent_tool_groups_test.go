package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
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
	defs := []providers.ToolDef{
		{Name: "Read", Description: "read a file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
		{Name: "Bash", Description: "run a command", InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)},
		{Name: "WebSearch", Description: "search the web", InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`)},
		{Name: "linear__issue", Description: "an issue", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "linear__team", Description: "a team", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	// Everything eager except WebSearch, so the search group's current cost sits
	// strictly below its all-full cost.
	tierOf := func(name string) string {
		if name == "WebSearch" {
			return tools.VisibilityHidden
		}
		return tools.VisibilityFull
	}
	groups, err := agentToolGroups(ctx, wsp, defs, tierOf)
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

	// Cost fields: relational assertions only — the estimator's absolute numbers
	// are an approximation and must not be pinned here.
	for _, g := range groups {
		if g.FullTokens <= 0 {
			t.Fatalf("group %s must report a positive full-tier cost: %+v", g.Key, g)
		}
		if g.CurrentTokens > g.FullTokens {
			t.Fatalf("group %s current cost exceeds all-full cost: %+v", g.Key, g)
		}
	}
	if files.CurrentTokens != files.FullTokens {
		t.Fatalf("an all-eager group must already cost its full price: %+v", files)
	}
	search, ok := byKey["group:search"]
	if !ok {
		t.Fatalf("group:search missing from %v", groups)
	}
	if search.CurrentTokens >= search.FullTokens {
		t.Fatalf("a hidden member must make the group cheaper than all-full: %+v", search)
	}
	if files.FullTokens <= mcpGroup.FullTokens {
		t.Fatalf("richer built-in schemas should outweigh the bare MCP ones: files=%d mcp=%d",
			files.FullTokens, mcpGroup.FullTokens)
	}
}
