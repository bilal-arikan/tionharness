package api

import (
	"net/http"
	"strings"
)

type setAgentReq struct {
	AgentID string `json:"agentId"`
}

// handleSetSessionAgent rebinds a chat session to a different agent. The chat UI
// selects the answering agent from a dropdown (the "@mention" routing was
// removed), so changing the selection persists here and every following turn is
// answered by this agent. The agent must exist in the workspace.
func (s *Server) handleSetSessionAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[setAgentReq](w, r)
	if !ok {
		return
	}
	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agentId required")
		return
	}

	ctx := r.Context()
	database := ws(r).DB
	if _, err := database.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	if _, err := database.GetAgent(ctx, agentID); writeDBError(w, err, "agent not found") {
		return
	}
	if err := database.SetSessionAgent(ctx, id, agentID); writeDBError(w, err, "") {
		return
	}
	// Cross-window sync: a sibling window's sidebar swaps the agent chip / next-
	// turn attribution immediately.
	emitSessionChange(ws(r), id, "agent")
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "agentId": agentID})
}
