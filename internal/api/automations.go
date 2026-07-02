package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

// defaultAutomationMaxIterations bounds a new automation's loop by default, so a
// user who leaves the field blank still gets a runaway brake.
const defaultAutomationMaxIterations = 50

func (s *Server) handleListAutomations(w http.ResponseWriter, r *http.Request) {
	autos, err := ws(r).DB.ListAutomations(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	if autos == nil {
		autos = []db.Automation{}
	}
	writeJSON(w, http.StatusOK, autos)
}

type automationReq struct {
	Name           string   `json:"name"`
	TriggerTag     string   `json:"triggerTag"`
	TargetAgentID  string   `json:"targetAgentId"`
	PromptTemplate string   `json:"promptTemplate"`
	SpawnTags      []string `json:"spawnTags"`
	Enabled        *bool    `json:"enabled"`
	MaxIterations  *int     `json:"maxIterations"`
	CooldownSec    *int     `json:"cooldownSec"`
}

func (s *Server) handleCreateAutomation(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[automationReq](w, r)
	if !ok {
		return
	}
	req.TriggerTag = strings.TrimSpace(req.TriggerTag)
	req.TargetAgentID = strings.TrimSpace(req.TargetAgentID)
	if req.TriggerTag == "" || req.TargetAgentID == "" || strings.TrimSpace(req.PromptTemplate) == "" {
		writeError(w, http.StatusBadRequest, "triggerTag, targetAgentId and promptTemplate are required")
		return
	}
	ctx := r.Context()
	if _, err := ws(r).DB.GetAgent(ctx, req.TargetAgentID); err != nil {
		writeError(w, http.StatusBadRequest, "target agent not found")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	maxIter := defaultAutomationMaxIterations
	if req.MaxIterations != nil {
		maxIter = *req.MaxIterations
	}
	cooldown := 0
	if req.CooldownSec != nil {
		cooldown = *req.CooldownSec
	}
	created, err := ws(r).DB.CreateAutomation(ctx, db.Automation{
		Name:           strings.TrimSpace(req.Name),
		TriggerTag:     req.TriggerTag,
		TargetAgentID:  req.TargetAgentID,
		PromptTemplate: req.PromptTemplate,
		SpawnTags:      req.SpawnTags,
		Enabled:        enabled,
		MaxIterations:  maxIter,
		CooldownSec:    cooldown,
	})
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("automation created", "automation", created.ID, "tag", created.TriggerTag, "agent", created.TargetAgentID)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateAutomation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[automationReq](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	cur, err := ws(r).DB.GetAutomation(ctx, id)
	if writeDBError(w, err, "automation not found") {
		return
	}
	if t := strings.TrimSpace(req.TriggerTag); t != "" {
		cur.TriggerTag = t
	}
	if a := strings.TrimSpace(req.TargetAgentID); a != "" {
		if _, err := ws(r).DB.GetAgent(ctx, a); err != nil {
			writeError(w, http.StatusBadRequest, "target agent not found")
			return
		}
		cur.TargetAgentID = a
	}
	if req.PromptTemplate != "" {
		cur.PromptTemplate = req.PromptTemplate
	}
	// Name and spawnTags are always taken from the request (they may be cleared).
	cur.Name = strings.TrimSpace(req.Name)
	cur.SpawnTags = req.SpawnTags
	if req.MaxIterations != nil {
		cur.MaxIterations = *req.MaxIterations
	}
	if req.CooldownSec != nil {
		cur.CooldownSec = *req.CooldownSec
	}
	if err := ws(r).DB.UpdateAutomation(ctx, cur); writeDBError(w, err, "") {
		return
	}
	if req.Enabled != nil {
		if err := ws(r).DB.SetAutomationEnabled(ctx, id, *req.Enabled); writeDBError(w, err, "") {
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "action": "updated"})
}

type toggleAutomationReq struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleToggleAutomation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[toggleAutomationReq](w, r)
	if !ok {
		return
	}
	if err := ws(r).DB.SetAutomationEnabled(r.Context(), id, req.Enabled); writeDBError(w, err, "automation not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": req.Enabled})
}

// handleResetAutomation clears the iteration counter so a maxed-out automation
// can run again without re-toggling.
func (s *Server) handleResetAutomation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ws(r).DB.ResetAutomationCount(r.Context(), id); writeDBError(w, err, "automation not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "action": "reset"})
}

func (s *Server) handleDeleteAutomation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ws(r).DB.DeleteAutomation(r.Context(), id); writeDBError(w, err, "automation not found") {
		return
	}
	s.logger.Info("automation deleted", "automation", id)
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}
