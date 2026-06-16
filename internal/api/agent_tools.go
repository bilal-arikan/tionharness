package api

import (
	"encoding/json"
	"net/http"

	"github.com/bilal/swarmgo/internal/providers"
)

// handleAgentTools returns the agent's tool access settings plus the catalog of
// tools it may pick from — the workspace-ACTIVE tools (built-ins + enabled MCP
// servers, minus the workspace denylist). The agent's allowedTools selects a
// subset of this catalog (empty = all active tools).
func (s *Server) handleAgentTools(w http.ResponseWriter, r *http.Request) {
	agent, err := ws(r).DB.GetAgent(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	catalog := ws(r).Runtime.ActiveToolCatalog(r.Context())
	if catalog == nil {
		catalog = []providers.ToolDef{}
	}
	var allowed []string
	_ = json.Unmarshal([]byte(agent.AllowedTools), &allowed)
	if allowed == nil {
		allowed = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mcpEnabled":   agent.MCPEnabled,
		"allowedTools": allowed,
		"catalog":      catalog,
	})
}

type setAgentToolsReq struct {
	MCPEnabled   bool     `json:"mcpEnabled"`
	AllowedTools []string `json:"allowedTools"`
}

// handleSetAgentTools updates the agent's tool access settings.
func (s *Server) handleSetAgentTools(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req setAgentToolsReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	allowedJSON, _ := json.Marshal(req.AllowedTools)
	if err := ws(r).DB.UpdateAgentTools(r.Context(), id, req.MCPEnabled, string(allowedJSON)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mcpEnabled": req.MCPEnabled, "allowedTools": req.AllowedTools})
}
