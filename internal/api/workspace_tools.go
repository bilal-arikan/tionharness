package api

import (
	"encoding/json"
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// workspaceTool is one entry in the workspace tools screen: a tool plus whether
// it is currently active (not in the workspace denylist) and its visibility tier.
// Source/Server/Label let the UI group tools by origin (built-in vs a specific MCP
// server) and show a clean, un-namespaced label; InputSchema drives the per-tool
// detail view; Examples surfaces concrete sample calls the schema alone can't show.
type workspaceTool struct {
	Name        string            `json:"name"`
	Label       string            `json:"label"`
	Description string            `json:"description"`
	Source      string            `json:"source"`             // "builtin" | "mcp"
	Server      string            `json:"server"`             // MCP server display name (empty for built-ins)
	Category    string            `json:"category,omitempty"` // functional group key for built-ins (empty for MCP — those group by server)
	Enabled     bool              `json:"enabled"`
	Visibility  string            `json:"visibility"` // "full" | "summary" | "name-only" | "hidden"
	InputSchema json.RawMessage   `json:"inputSchema,omitempty"`
	Examples    []json.RawMessage `json:"examples,omitempty"` // concrete sample calls (ToolDef.Examples)
}

// handleWorkspaceTools returns the full workspace tool catalog (built-ins + all
// enabled MCP servers' tools), each marked active/inactive per the workspace
// denylist and tagged with its effective visibility tier. Drives the tools screen.
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

	// visibility reflects the EFFECTIVE tier per tool (code defaults + the workspace
	// per-tool overrides), so the screen's tier selector matches what the agent sees.
	catalog, visibility := ws(r).Runtime.WorkspaceToolCatalogWithState(r.Context())
	out := make([]workspaceTool, 0, len(catalog))
	for _, t := range catalog {
		wt := workspaceTool{
			Name:        t.Name,
			Label:       t.Name,
			Description: t.Description,
			Source:      "builtin",
			Category:    tools.CategoryOf(t.Name),
			Enabled:     !disabled[t.Name],
			Visibility:  visibility[t.Name],
			InputSchema: t.InputSchema,
			Examples:    t.Examples,
		}
		// MCP tools are namespaced "<server>__<tool>"; recover origin and label.
		if ns, tool, ok := mcp.SplitNamespaced(t.Name); ok {
			wt.Source = "mcp"
			wt.Label = tool
			wt.Category = "" // MCP tools group by server, not functional category
			if name, found := serverByNS[ns]; found {
				wt.Server = name
			} else {
				wt.Server = ns
			}
		}
		out = append(out, wt)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tools":          out,
		"disabledTools":  cfg.DisabledTools,
		"toolVisibility": cfg.ToolVisibility,
	})
}

// setWorkspaceToolsReq carries the independent tool override maps. Each field is a
// pointer so the client can update one without resupplying (and clearing) the
// other — nil means "leave unchanged", a present (possibly empty) value replaces.
// ToolVisibility maps a tool name to one of "full" | "summary" | "name-only" |
// "hidden"; a tool absent from the map uses its code default.
type setWorkspaceToolsReq struct {
	DisabledTools  *[]string          `json:"disabledTools"`
	ToolVisibility *map[string]string `json:"toolVisibility"`
}

// handleSetWorkspaceTools updates the workspace tool denylist and/or the per-tool
// visibility map. Only the fields present in the request change. Invalid
// visibility values are rejected so a typo can't silently leave a tool at its
// default.
func (s *Server) handleSetWorkspaceTools(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[setWorkspaceToolsReq](w, r)
	if !ok {
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
	if req.ToolVisibility != nil {
		for name, tier := range *req.ToolVisibility {
			if !validVisibility(tier) {
				writeError(w, http.StatusBadRequest, "invalid visibility for "+name+": "+tier)
				return
			}
		}
		cfg.ToolVisibility = *req.ToolVisibility
	}
	if err := ws(r).DB.SetWorkspaceToolConfig(r.Context(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"disabledTools":  cfg.DisabledTools,
		"toolVisibility": cfg.ToolVisibility,
	})
}

// validVisibility reports whether tier is one of the four allowed values.
func validVisibility(tier string) bool {
	switch tier {
	case tools.VisibilityFull, tools.VisibilitySummary, tools.VisibilityNameOnly, tools.VisibilityHidden:
		return true
	default:
		return false
	}
}
