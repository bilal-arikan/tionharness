package api

import (
	"net/http"

	"github.com/bilal/swarmgo/internal/db"
)

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := ws(r).DB.ListAgents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if agents == nil {
		agents = []db.Agent{}
	}
	writeJSON(w, http.StatusOK, agents)
}

type createAgentReq struct {
	Name         string `json:"name"`
	Soul         string `json:"soul"`
	Identity     string `json:"identity"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	PlanningMode string `json:"planningMode"`
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var req createAgentReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Provider == "" {
		req.Provider = "claude-cli" // default: use local Claude Code login, no API key
	}

	agent, err := ws(r).DB.CreateAgent(r.Context(), db.Agent{
		Name:         req.Name,
		Soul:         req.Soul,
		Identity:     req.Identity,
		Provider:     req.Provider,
		Model:        req.Model,
		PlanningMode: req.PlanningMode,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, agent)
}
