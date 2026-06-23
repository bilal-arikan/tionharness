package api

import (
	"encoding/json"
	"net/http"

	"github.com/bilal-arikan/swarmgo/internal/mcp"
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
	Hidden      bool            `json:"hidden"` // load-on-demand (lazy): not shipped every turn
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

	// hidden reflects the EFFECTIVE load-on-demand state per tool (code defaults
	// like the self-management suite + the workspace Hidden/Shown overrides), not
	// just the HiddenTools list — so the "Gizli" chip matches what the agent sees.
	catalog, lazy := ws(r).Runtime.WorkspaceToolCatalogWithState(r.Context())
	out := make([]workspaceTool, 0, len(catalog))
	for _, t := range catalog {
		wt := workspaceTool{
			Name:        t.Name,
			Label:       t.Name,
			Description: t.Description,
			Source:      "builtin",
			Enabled:     !disabled[t.Name],
			Hidden:      lazy[t.Name],
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
		"hiddenTools":   cfg.HiddenTools,
		"shownTools":    cfg.ShownTools,
	})
}

// setWorkspaceToolsReq carries the independent tool override lists. Each field is
// a pointer so the client can update one without resupplying (and clearing) the
// others — nil means "leave unchanged", a present (possibly empty) array
// replaces. HiddenTools forces a tool load-on-demand; ShownTools forces a
// default-hidden tool (e.g. self-management) back into the every-turn context.
type setWorkspaceToolsReq struct {
	DisabledTools *[]string `json:"disabledTools"`
	HiddenTools   *[]string `json:"hiddenTools"`
	ShownTools    *[]string `json:"shownTools"`
}

// handleSetWorkspaceTools updates the workspace tool denylist and/or the
// hidden/shown override lists. Only the fields present in the request change.
func (s *Server) handleSetWorkspaceTools(w http.ResponseWriter, r *http.Request) {
	var req setWorkspaceToolsReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cfg, err := ws(r).DB.GetWorkspaceToolConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req.DisabledTools != nil {
		cfg.DisabledTools = *req.DisabledTools
	}
	if req.HiddenTools != nil {
		cfg.HiddenTools = *req.HiddenTools
	}
	if req.ShownTools != nil {
		cfg.ShownTools = *req.ShownTools
	}
	if err := ws(r).DB.SetWorkspaceToolConfig(r.Context(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"disabledTools": cfg.DisabledTools,
		"hiddenTools":   cfg.HiddenTools,
		"shownTools":    cfg.ShownTools,
	})
}
