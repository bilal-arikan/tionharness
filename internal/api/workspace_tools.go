package api

import (
	"encoding/json"
	"net/http"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/mcp"
)

// workspaceTool is one entry in the workspace tools screen: a tool plus whether
// it is currently active (not in the workspace denylist). Source/Server/Label
// let the UI group tools by origin (built-in vs a specific MCP server) and show
// a clean, un-namespaced label; InputSchema drives the per-tool detail view.
type workspaceTool struct {
	Name        string          `json:"name"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
	Source      string          `json:"source"` // "builtin" | "mcp"
	Server      string          `json:"server"` // MCP server display name (empty for built-ins)
	Enabled     bool            `json:"enabled"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// handleWorkspaceTools returns the full workspace tool catalog (built-ins + all
// enabled MCP servers' tools), each marked active/inactive per the workspace
// denylist. This drives the workspace-wide tools screen.
func (s *Server) handleWorkspaceTools(w http.ResponseWriter, r *http.Request) {
	cfg, err := ws(r).DB.GetWorkspaceToolConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	disabled := map[string]bool{}
	for _, n := range cfg.DisabledTools {
		disabled[n] = true
	}

	// Map each MCP server's sanitized namespace prefix back to its display name
	// so MCP tools can be grouped/labelled by the human-readable server name.
	serverByNS := map[string]string{}
	if servers, err := ws(r).DB.ListEnabledMCPServers(r.Context()); err == nil {
		for _, m := range servers {
			if ns, _, ok := mcp.SplitNamespaced(mcp.NamespaceTool(m.Name, "x")); ok {
				serverByNS[ns] = m.Name
			}
		}
	}

	catalog := ws(r).Runtime.WorkspaceToolCatalog(r.Context())
	out := make([]workspaceTool, 0, len(catalog))
	for _, t := range catalog {
		wt := workspaceTool{
			Name:        t.Name,
			Label:       t.Name,
			Description: t.Description,
			Source:      "builtin",
			Enabled:     !disabled[t.Name],
			InputSchema: t.InputSchema,
		}
		// MCP tools are namespaced "<server>__<tool>"; recover origin and label.
		if ns, tool, ok := mcp.SplitNamespaced(t.Name); ok {
			wt.Source = "mcp"
			wt.Label = tool
			if name, found := serverByNS[ns]; found {
				wt.Server = name
			} else {
				wt.Server = ns
			}
		}
		out = append(out, wt)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tools":         out,
		"disabledTools": cfg.DisabledTools,
	})
}

type setWorkspaceToolsReq struct {
	DisabledTools []string `json:"disabledTools"`
}

// handleSetWorkspaceTools replaces the workspace tool denylist (tools switched
// off for the whole workspace).
func (s *Server) handleSetWorkspaceTools(w http.ResponseWriter, r *http.Request) {
	var req setWorkspaceToolsReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.DisabledTools == nil {
		req.DisabledTools = []string{}
	}
	if err := ws(r).DB.SetWorkspaceToolConfig(r.Context(), db.WorkspaceToolConfig{DisabledTools: req.DisabledTools}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"disabledTools": req.DisabledTools})
}
