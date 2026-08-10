package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// MCP server self-management tools let an agent read and manage the Model
// Context Protocol servers in its workspace — so an agent can wire up a new
// external tool source (a stdio subprocess or an sse/http endpoint) for itself
// and later agents. Newly created/enabled servers are picked up on the next
// agent turn (the tool catalog is rebuilt per turn from enabled servers).
// No provenance gate: list/create/toggle/delete on any server (user- or
// agent-created). MCPServer.CreatedBy is still stamped for provenance/display.

type mcpDeps struct {
	db      *db.DB
	actorID string
}

// ---- list_mcp_servers ----

// ListMCPServersTool returns the workspace MCP servers as a compact list.
type ListMCPServersTool struct{ d mcpDeps }

// NewListMCPServersTool constructs list_mcp_servers.
func NewListMCPServersTool(database *db.DB, actorID string) ListMCPServersTool {
	return ListMCPServersTool{d: mcpDeps{db: database, actorID: actorID}}
}

func (ListMCPServersTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_mcp_servers",
		Description: "List the MCP (Model Context Protocol) servers configured in this workspace. Returns id, name, " +
			"transport (stdio|sse|http), command/url, enabled, and whether you created it (and may therefore delete " +
			"it). Enabled servers' tools are available to agents on their next turn. Results are PAGINATED: pass " +
			"limit (default 20, max 100) and offset to page; the reply reports total and hasMore, and you reach " +
			"the next page with offset += limit. Filters: transport (case-insensitive substring), enabled. Sort: " +
			"updated_desc (default), updated_asc, created_desc, created_asc, name_asc, name_desc — servers are " +
			"never edited in place, so updated_* sorts by creation time.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "transport": { "type": "string", "description": "Only servers whose transport contains this substring (case-insensitive)." },
    "enabled": { "type": "boolean", "description": "Only enabled or only disabled servers (omit for both)." },
    "sort": { "type": "string", "enum": ["updated_desc", "updated_asc", "created_desc", "created_asc", "name_asc", "name_desc"], "description": "Result ordering (default updated_desc; updated maps to creation time — servers are immutable)." },
    "limit": { "type": "integer", "description": "Max servers per page (default 20, max 100)." },
    "offset": { "type": "integer", "description": "How many matching servers to skip before this page (default 0)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t ListMCPServersTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Transport string `json:"transport"`
		Enabled   *bool  `json:"enabled"`
		Sort      string `json:"sort"`
		Limit     int    `json:"limit"`
		Offset    int    `json:"offset"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	limit, offset := PageArgs(in.Limit, in.Offset)

	servers, err := t.d.db.ListMCPServers(ctx)
	if err != nil {
		return "", err
	}
	transport := strings.ToLower(strings.TrimSpace(in.Transport))
	matches := make([]db.MCPServer, 0, len(servers))
	for _, m := range servers {
		if transport != "" && !strings.Contains(strings.ToLower(m.Transport), transport) {
			continue
		}
		if in.Enabled != nil && m.Enabled != *in.Enabled {
			continue
		}
		matches = append(matches, m)
	}

	field, asc, err := SortOrder(in.Sort)
	if err != nil {
		return "", err
	}
	// Servers are never edited in place (no updated timestamp), so the updated_*
	// keys sort by CreatedAt — the same value as created_*.
	less, err := SortByField(matches, field, asc,
		func(m db.MCPServer) int64 { return m.CreatedAt },
		func(m db.MCPServer) int64 { return m.CreatedAt },
		func(m db.MCPServer) string { return m.Name },
		func(m db.MCPServer) string { return m.ID },
	)
	if err != nil {
		return "", err
	}
	sort.SliceStable(matches, less)

	page, total := SlicePage(matches, offset, limit)
	type row struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Description    string `json:"description,omitempty"`
		Transport      string `json:"transport"`
		Command        string `json:"command,omitempty"`
		URL            string `json:"url,omitempty"`
		Enabled        bool   `json:"enabled"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(page))
	for _, m := range page {
		out = append(out, row{
			ID: m.ID, Name: m.Name, Description: m.Description, Transport: m.Transport,
			Command: m.Command, URL: m.URL, Enabled: m.Enabled,
			CreatedByAgent: m.CreatedBy != "",
		})
	}
	return pageResult(out, total, offset, limit)
}

// ---- create_mcp_server ----

// CreateMCPServerTool adds an MCP server to the workspace.
type CreateMCPServerTool struct{ d mcpDeps }

// NewCreateMCPServerTool constructs create_mcp_server.
func NewCreateMCPServerTool(database *db.DB, actorID string) CreateMCPServerTool {
	return CreateMCPServerTool{d: mcpDeps{db: database, actorID: actorID}}
}

func (CreateMCPServerTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "create_mcp_server",
		Description: "Add an MCP (Model Context Protocol) server to this workspace. For transport=stdio set command (executable) and optionally args (JSON array string) and env (JSON object string). For transport=sse or http set url. The server is enabled and tagged as created by you; its tools become available to agents on their next turn. Returns the new server id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Display name"},
				"description":{"type":"string","description":"Short one-liner about what this server is for and when to use it (rides the load-on-demand catalog's per-server summary, e.g. 'code knowledge graph; prefer over grep for code search')"},
				"transport":{"type":"string","enum":["stdio","sse","http"]},
				"command":{"type":"string","description":"Executable for transport=stdio"},
				"args":{"type":"string","description":"JSON array of arguments, e.g. [\"-y\",\"@scope/pkg\"]"},
				"url":{"type":"string","description":"Endpoint for transport=sse/http"},
				"env":{"type":"string","description":"JSON object of environment variables, e.g. {\"API_KEY\":\"...\"}"}
			},
			"required":["name","transport"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			// stdio: command + args is a JSON ARRAY STRING (escaped).
			json.RawMessage(`{"name":"filesystem","transport":"stdio","command":"npx","args":"[\"-y\",\"@modelcontextprotocol/server-filesystem\",\"/data\"]"}`),
			// remote endpoint: set url instead of command.
			json.RawMessage(`{"name":"my-api","transport":"http","url":"https://mcp.example.com/sse"}`),
			// stdio with env (a JSON OBJECT STRING).
			json.RawMessage(`{"name":"github","transport":"stdio","command":"npx","args":"[\"-y\",\"@modelcontextprotocol/server-github\"]","env":"{\"GITHUB_TOKEN\":\"ghp_xxx\"}"}`),
		},
	}
}

func (t CreateMCPServerTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Transport   string `json:"transport"`
		Command     string `json:"command"`
		Args        string `json:"args"`
		URL         string `json:"url"`
		Env         string `json:"env"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	switch in.Transport {
	case db.MCPTransportStdio:
		if strings.TrimSpace(in.Command) == "" {
			return "", fmt.Errorf("command is required for transport=stdio")
		}
	case db.MCPTransportSSE, db.MCPTransportHTTP:
		if strings.TrimSpace(in.URL) == "" {
			return "", fmt.Errorf("url is required for transport=%s", in.Transport)
		}
	default:
		return "", fmt.Errorf("invalid transport %q (stdio|sse|http)", in.Transport)
	}
	// Validate JSON-shaped fields up front so a bad value fails clearly here.
	if s := strings.TrimSpace(in.Args); s != "" && !json.Valid([]byte(s)) {
		return "", fmt.Errorf("args must be a JSON array string")
	}
	if s := strings.TrimSpace(in.Env); s != "" && !json.Valid([]byte(s)) {
		return "", fmt.Errorf("env must be a JSON object string")
	}
	created, err := t.d.db.CreateMCPServer(ctx, db.MCPServer{
		Name:        in.Name,
		Description: strings.TrimSpace(in.Description),
		Transport:   in.Transport,
		Command:     in.Command,
		Args:        in.Args,
		URL:         in.URL,
		EnvConfig:   in.Env,
		Enabled:     true,
		Scope:       "shared",
		CreatedBy:   t.d.actorID,
	})
	if err != nil {
		return "", fmt.Errorf("create mcp server: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": created.ID, "action": "created"})
	return string(b), nil
}

// ---- toggle_mcp_server ----

// ToggleMCPServerTool enables or disables an MCP server (any server).
type ToggleMCPServerTool struct{ d mcpDeps }

// NewToggleMCPServerTool constructs toggle_mcp_server.
func NewToggleMCPServerTool(database *db.DB, actorID string) ToggleMCPServerTool {
	return ToggleMCPServerTool{d: mcpDeps{db: database, actorID: actorID}}
}

func (ToggleMCPServerTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "toggle_mcp_server",
		Description: "Enable or disable an MCP server. Disabling removes its tools from agents on their next turn. Allowed on any server.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The server id (see list_mcp_servers)"},
				"enabled":{"type":"boolean"}
			},
			"required":["id","enabled"],
			"additionalProperties":false
		}`),
	}
}

func (t ToggleMCPServerTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if err := t.d.db.SetMCPServerEnabled(ctx, in.ID, in.Enabled); err != nil {
		return "", fmt.Errorf("toggle mcp server: %w", err)
	}
	b, _ := json.Marshal(map[string]any{"id": in.ID, "enabled": in.Enabled, "action": "toggled"})
	return string(b), nil
}

// ---- delete_mcp_server ----

// DeleteMCPServerTool removes an MCP server (user- or agent-created).
type DeleteMCPServerTool struct{ d mcpDeps }

// NewDeleteMCPServerTool constructs delete_mcp_server.
func NewDeleteMCPServerTool(database *db.DB, actorID string) DeleteMCPServerTool {
	return DeleteMCPServerTool{d: mcpDeps{db: database, actorID: actorID}}
}

func (DeleteMCPServerTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_mcp_server",
		Description: "Delete an MCP server (user- or agent-created). Pass the server id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The server id (see list_mcp_servers)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteMCPServerTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if _, err := t.d.db.GetMCPServer(ctx, in.ID); err != nil {
		return "", fmt.Errorf("no mcp server with id %q (use list_mcp_servers)", in.ID)
	}
	if err := t.d.db.DeleteMCPServer(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete mcp server: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}
