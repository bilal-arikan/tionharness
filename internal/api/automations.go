package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
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
	Name            string   `json:"name"`
	TriggerKind     string   `json:"triggerKind"`
	TriggerTag      string   `json:"triggerTag"`
	BoardOp         string   `json:"boardOp"`
	BoardFromState  string   `json:"boardFromState"`
	BoardToState    string   `json:"boardToState"`
	BoardPriority   *int     `json:"boardPriority"`
	BoardExclusive  *bool    `json:"boardExclusive"`
	BoardAction     string   `json:"boardAction"`
	TokenScope      string   `json:"tokenScope"`
	TokenThreshold  *int     `json:"tokenThreshold"`
	CounterMetric   string   `json:"counterMetric"`
	CounterInterval *int     `json:"counterInterval"`
	TargetAgentID   string   `json:"targetAgentId"`
	FlowID          string   `json:"flowId"`
	PromptTemplate  string   `json:"promptTemplate"`
	SpawnTags       []string `json:"spawnTags"`
	Enabled         *bool    `json:"enabled"`
	MaxIterations   *int     `json:"maxIterations"`
	CooldownSec     *int     `json:"cooldownSec"`
	ExpiresAt       *int64   `json:"expiresAt"`
}

func (s *Server) handleCreateAutomation(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[automationReq](w, r)
	if !ok {
		return
	}
	req.TriggerKind = strings.TrimSpace(req.TriggerKind)
	req.TriggerTag = strings.TrimSpace(req.TriggerTag)
	req.BoardOp = strings.TrimSpace(req.BoardOp)
	req.BoardFromState = strings.TrimSpace(req.BoardFromState)
	req.BoardToState = strings.TrimSpace(req.BoardToState)
	req.BoardAction = strings.TrimSpace(req.BoardAction)
	req.TokenScope = strings.TrimSpace(req.TokenScope)
	req.CounterMetric = strings.TrimSpace(req.CounterMetric)
	req.TargetAgentID = strings.TrimSpace(req.TargetAgentID)
	req.FlowID = strings.TrimSpace(req.FlowID)
	if strings.TrimSpace(req.PromptTemplate) == "" {
		writeError(w, http.StatusBadRequest, "promptTemplate is required")
		return
	}
	// Trigger-kind-specific requirements: board fires on card changes (no tag),
	// token on spend crossings (no tag), tag (the default) needs a trigger tag.
	tokenThreshold := 0
	counterInterval := 0
	switch req.TriggerKind {
	case db.TriggerBoard:
		if !db.ValidBoardOp(req.BoardOp) {
			writeError(w, http.StatusBadRequest, "invalid boardOp")
			return
		}
		if !db.ValidBoardAction(req.BoardAction) {
			writeError(w, http.StatusBadRequest, "invalid boardAction (spawn|archive)")
			return
		}
	case db.TriggerToken:
		if !db.ValidTokenScope(req.TokenScope) {
			writeError(w, http.StatusBadRequest, "invalid tokenScope (session|workspace)")
			return
		}
		if req.TokenThreshold == nil {
			writeError(w, http.StatusBadRequest, "tokenThreshold is required for token automations")
			return
		}
		if err := db.ValidateTokenThreshold(*req.TokenThreshold); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		tokenThreshold = *req.TokenThreshold
	case db.TriggerCounter:
		if !db.ValidCounterMetric(req.CounterMetric) {
			writeError(w, http.StatusBadRequest, "invalid counterMetric (message|tool)")
			return
		}
		if req.CounterInterval == nil {
			writeError(w, http.StatusBadRequest, "counterInterval is required for counter automations")
			return
		}
		if err := db.ValidateCounterInterval(*req.CounterInterval); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		counterInterval = *req.CounterInterval
	default:
		if req.TriggerTag == "" {
			writeError(w, http.StatusBadRequest, "triggerTag is required for tag automations")
			return
		}
	}
	ctx := r.Context()
	// A board automation whose action is "archive" performs bookkeeping with no
	// LLM call, so it needs no target. Every other automation targets EITHER a flow
	// or a single agent.
	archiveAction := req.TriggerKind == db.TriggerBoard && req.BoardAction == db.BoardActionArchive
	if archiveAction {
		// no target required; ignore any flow/agent sent
	} else if req.FlowID != "" {
		if _, err := ws(r).DB.GetFlow(ctx, req.FlowID); err != nil {
			writeError(w, http.StatusBadRequest, "target flow not found")
			return
		}
	} else {
		if req.TargetAgentID == "" {
			writeError(w, http.StatusBadRequest, "targetAgentId or flowId is required")
			return
		}
		if _, err := ws(r).DB.GetAgent(ctx, req.TargetAgentID); err != nil {
			writeError(w, http.StatusBadRequest, "target agent not found")
			return
		}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	maxIter := defaultAutomationMaxIterations
	if req.MaxIterations != nil {
		maxIter = *req.MaxIterations
		// Shared with the agent-tool path (tools.create_automation) via db, so the
		// two entry points cannot diverge. See db.ValidateMaxIterations.
		if err := db.ValidateMaxIterations(maxIter); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	cooldown := 0
	if req.CooldownSec != nil {
		cooldown = *req.CooldownSec
	}
	var expiresAt int64
	if req.ExpiresAt != nil {
		expiresAt = *req.ExpiresAt
	}
	boardPriority := 0
	if req.BoardPriority != nil {
		boardPriority = *req.BoardPriority
	}
	boardExclusive := false
	if req.BoardExclusive != nil {
		boardExclusive = *req.BoardExclusive
	}
	auto := db.Automation{
		Name:            strings.TrimSpace(req.Name),
		TriggerKind:     req.TriggerKind,
		TriggerTag:      req.TriggerTag,
		BoardOp:         req.BoardOp,
		BoardFromState:  req.BoardFromState,
		BoardToState:    req.BoardToState,
		BoardPriority:   boardPriority,
		BoardExclusive:  boardExclusive,
		BoardAction:     req.BoardAction,
		TokenScope:      req.TokenScope,
		TokenThreshold:  tokenThreshold,
		CounterMetric:   req.CounterMetric,
		CounterInterval: counterInterval,
		TargetAgentID:   req.TargetAgentID,
		FlowID:          req.FlowID,
		PromptTemplate:  req.PromptTemplate,
		SpawnTags:       req.SpawnTags,
		Enabled:         enabled,
		MaxIterations:   maxIter,
		CooldownSec:     cooldown,
		ExpiresAt:       expiresAt,
	}
	// Shared shape backstop: same contract as the agent tool + update paths.
	if err := db.ValidateAutomationShape(auto); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := ws(r).DB.CreateAutomation(ctx, auto)
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
	// Trigger kind + board filters. TriggerKind is only applied when the request
	// specifies it (a partial patch like spawnTags-only sends "" and must not flip
	// a board automation back to a tag one). A full edit always sends the kind, so
	// the board filters are re-applied together with it (empty = "any" / cleared).
	if k := strings.TrimSpace(req.TriggerKind); k != "" {
		if k == db.TriggerBoard && !db.ValidBoardOp(strings.TrimSpace(req.BoardOp)) {
			writeError(w, http.StatusBadRequest, "invalid boardOp")
			return
		}
		if k == db.TriggerBoard && !db.ValidBoardAction(strings.TrimSpace(req.BoardAction)) {
			writeError(w, http.StatusBadRequest, "invalid boardAction (spawn|archive)")
			return
		}
		if k == db.TriggerToken && !db.ValidTokenScope(strings.TrimSpace(req.TokenScope)) {
			writeError(w, http.StatusBadRequest, "invalid tokenScope (session|workspace)")
			return
		}
		if k == db.TriggerCounter && !db.ValidCounterMetric(strings.TrimSpace(req.CounterMetric)) {
			writeError(w, http.StatusBadRequest, "invalid counterMetric (message|tool)")
			return
		}
		cur.TriggerKind = k
		cur.BoardOp = strings.TrimSpace(req.BoardOp)
		cur.BoardFromState = strings.TrimSpace(req.BoardFromState)
		cur.BoardToState = strings.TrimSpace(req.BoardToState)
		if k == db.TriggerBoard {
			cur.BoardAction = strings.TrimSpace(req.BoardAction)
		}
		if k == db.TriggerToken {
			cur.TokenScope = strings.TrimSpace(req.TokenScope)
		}
		if k == db.TriggerCounter {
			cur.CounterMetric = strings.TrimSpace(req.CounterMetric)
		}
	}
	// TokenThreshold is a pointer field: absent in a partial patch means "leave as
	// stored"; when present it is validated. A rule that ends up token-triggered
	// must carry a valid threshold (guarded after the patches below).
	if req.TokenThreshold != nil {
		if err := db.ValidateTokenThreshold(*req.TokenThreshold); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cur.TokenThreshold = *req.TokenThreshold
	}
	if cur.TriggerKind == db.TriggerToken {
		if err := db.ValidateTokenThreshold(cur.TokenThreshold); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	// CounterInterval is a pointer field: absent in a partial patch means "leave as
	// stored"; when present it is validated. A rule that ends up counter-triggered
	// must carry a valid interval (guarded after the patches).
	if req.CounterInterval != nil {
		if err := db.ValidateCounterInterval(*req.CounterInterval); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cur.CounterInterval = *req.CounterInterval
	}
	if cur.TriggerKind == db.TriggerCounter {
		if err := db.ValidateCounterInterval(cur.CounterInterval); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	// Targeting: apply only when the request specifies a target, so partial
	// updates (e.g. spawnTags-only) don't wipe it. Setting a flow switches the
	// automation to flow-backed and clears the agent, and vice versa.
	if f := strings.TrimSpace(req.FlowID); f != "" {
		if _, err := ws(r).DB.GetFlow(ctx, f); err != nil {
			writeError(w, http.StatusBadRequest, "target flow not found")
			return
		}
		cur.FlowID = f
		cur.TargetAgentID = ""
	} else if a := strings.TrimSpace(req.TargetAgentID); a != "" {
		if _, err := ws(r).DB.GetAgent(ctx, a); err != nil {
			writeError(w, http.StatusBadRequest, "target agent not found")
			return
		}
		cur.TargetAgentID = a
		cur.FlowID = ""
	}
	if req.PromptTemplate != "" {
		cur.PromptTemplate = req.PromptTemplate
	}
	// Name and spawnTags are always taken from the request (they may be cleared).
	cur.Name = strings.TrimSpace(req.Name)
	cur.SpawnTags = req.SpawnTags
	if req.MaxIterations != nil {
		if err := db.ValidateMaxIterations(*req.MaxIterations); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cur.MaxIterations = *req.MaxIterations
	}
	if req.CooldownSec != nil {
		cur.CooldownSec = *req.CooldownSec
	}
	if req.ExpiresAt != nil {
		cur.ExpiresAt = *req.ExpiresAt
	}
	// Board ordering/exclusivity are pointer fields: absent in a partial patch
	// means "leave as stored", so a spawnTags-only edit can't silently reset a
	// column's owner back to 0/false.
	if req.BoardPriority != nil {
		cur.BoardPriority = *req.BoardPriority
	}
	if req.BoardExclusive != nil {
		cur.BoardExclusive = *req.BoardExclusive
	}
	// Final backstop on the merged result: the same shape check the agent tool and
	// create paths run, so a partial patch can't leave the rule unable to fire.
	if err := db.ValidateAutomationShape(cur); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
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
