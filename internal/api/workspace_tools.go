package api

import (
	"net/http"

	"github.com/bilal/swarmgo/internal/db"
)

// workspaceTool is one entry in the workspace tools screen: a tool plus whether
// it is currently active (not in the workspace denylist).
type workspaceTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
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
	catalog := ws(r).Runtime.WorkspaceToolCatalog(r.Context())
	out := make([]workspaceTool, 0, len(catalog))
	for _, t := range catalog {
		out = append(out, workspaceTool{
			Name:        t.Name,
			Description: t.Description,
			Enabled:     !disabled[t.Name],
		})
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
