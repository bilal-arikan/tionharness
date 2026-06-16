package api

import (
	"net/http"

	"github.com/bilal/swarmgo/internal/db"
)

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := ws(r).DB.ListAgents(r.Context())
	if writeDBError(w, err, "") {
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
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	PlanningMode  string `json:"planningMode"`
	ThinkingLevel string `json:"thinkingLevel"`
	Avatar        string `json:"avatar"`
	Color         string `json:"color"`
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

	// Fall back to defaults for any blank field. Precedence: request value →
	// per-workspace override → application-global default → built-in last resort.
	cfg := s.settings.Get()
	wsCfg := ws(r).Settings()
	if req.Provider == "" {
		req.Provider = wsCfg.DefaultProvider
	}
	if req.Provider == "" {
		req.Provider = cfg.DefaultProvider
	}
	if req.Provider == "" {
		req.Provider = "claude-cli" // last-resort: local Claude Code login, no API key
	}
	if req.Model == "" {
		req.Model = wsCfg.DefaultModel
	}
	if req.Model == "" {
		req.Model = cfg.DefaultModel
	}

	agent, err := ws(r).DB.CreateAgent(r.Context(), db.Agent{
		Name:         req.Name,
		Soul:         req.Soul,
		Identity:     req.Identity,
		Provider:      req.Provider,
		Model:         req.Model,
		PlanningMode:  req.PlanningMode,
		ThinkingLevel: req.ThinkingLevel,
	})
	if writeDBError(w, err, "") {
		return
	}

	if req.Avatar != "" {
		_, _ = ws(r).DB.UpdateAgent(r.Context(), agent.ID, db.AgentProfilePatch{Avatar: &req.Avatar})
		agent.Avatar = req.Avatar
	}
	if req.Color != "" {
		_, _ = ws(r).DB.UpdateAgent(r.Context(), agent.ID, db.AgentProfilePatch{Color: &req.Color})
		agent.Color = req.Color
	}

	// Seed the agent's daily spend caps from the configured defaults.
	if cfg.DefaultDailyCallLimit > 0 || cfg.DefaultDailyTokenLimit > 0 {
		if err := ws(r).DB.UpdateBudget(r.Context(), agent.ID, cfg.DefaultDailyCallLimit, cfg.DefaultDailyTokenLimit); err != nil {
			s.logger.Warn("apply default budget failed", "agent", agent.ID, "error", err)
		} else {
			agent.DailyCallLimit = cfg.DefaultDailyCallLimit
			agent.DailyTokenLimit = cfg.DefaultDailyTokenLimit
		}
	}
	writeJSON(w, http.StatusCreated, agent)
}

type updateAgentReq struct {
	Name         *string `json:"name"`
	Soul         *string `json:"soul"`
	Identity     *string `json:"identity"`
	Provider      *string `json:"provider"`
	Model         *string `json:"model"`
	PlanningMode  *string `json:"planningMode"`
	ThinkingLevel *string `json:"thinkingLevel"`
	Avatar        *string `json:"avatar"`
	Color         *string `json:"color"`
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	var req updateAgentReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name != nil && *req.Name == "" {
		writeError(w, http.StatusBadRequest, "name cannot be empty")
		return
	}

	agent, err := ws(r).DB.UpdateAgent(r.Context(), r.PathValue("id"), db.AgentProfilePatch{
		Name:         req.Name,
		Soul:         req.Soul,
		Identity:     req.Identity,
		Provider:      req.Provider,
		Model:         req.Model,
		PlanningMode:  req.PlanningMode,
		ThinkingLevel: req.ThinkingLevel,
		Avatar:        req.Avatar,
		Color:         req.Color,
	})
	if writeDBError(w, err, "agent not found") {
		return
	}
	writeJSON(w, http.StatusOK, agent)
}
