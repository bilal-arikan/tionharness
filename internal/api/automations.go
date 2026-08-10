package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// defaultAutomationMaxIterations bounds a new automation's loop by default, so a
// user who leaves the field blank still gets a runaway brake.
const defaultAutomationMaxIterations = 50

// handleAutomationLiveStats returns the live workspace-wide metrics that
// workspace-scoped automations key on, so the automation screen can show "where
// am I relative to the next fire" in each lane header: today's cumulative token
// spend (what a token automation watches) and the cumulative message/tool counts
// (what a counter automation watches). Cheap to compute — the same aggregates the
// engine reads on each crossing.
func (s *Server) handleAutomationLiveStats(w http.ResponseWriter, r *http.Request) {
	database := ws(r).DB
	writeJSON(w, http.StatusOK, map[string]int64{
		"tokensToday": database.WorkspaceTokensToday(r.Context()),
		"messages":    database.WorkspaceCounterTotal(db.CounterMetricMessage),
		"tools":       database.WorkspaceCounterTotal(db.CounterMetricTool),
	})
}

func (s *Server) handleListAutomations(w http.ResponseWriter, r *http.Request) {
	autos, err := ws(r).DB.ListAutomations(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	if autos == nil {
		autos = []db.Automation{}
	}
	q := r.URL.Query()
	limit, offset, field, asc, listing, err := listQueryParams(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	enabled, hasEnabled, err := boolQuery(q, "enabled")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	triggerKind := q.Get("triggerKind")
	switch triggerKind {
	case "", "tag", "board", "token", "counter":
	default:
		writeError(w, http.StatusBadRequest, "triggerKind must be one of tag, board, token, counter")
		return
	}
	target := q.Get("targetAgentId")
	matches := make([]db.Automation, 0, len(autos))
	for _, a := range autos {
		if hasEnabled && a.Enabled != *enabled {
			continue
		}
		if triggerKind != "" {
			kind := a.TriggerKind
			if kind == "" {
				kind = db.TriggerTag
			}
			if kind != triggerKind {
				continue
			}
		}
		if target != "" && a.TargetAgentID != target {
			continue
		}
		matches = append(matches, a)
	}
	if !listing {
		writeJSON(w, http.StatusOK, matches)
		return
	}
	if field != "" {
		less, err := tools.SortByField(matches, field, asc,
			func(a db.Automation) int64 { return a.UpdatedAt },
			func(a db.Automation) int64 { return a.CreatedAt },
			func(a db.Automation) string { return a.Name },
			func(a db.Automation) string { return a.ID })
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sort.SliceStable(matches, less)
	}
	page, total := tools.SlicePage(matches, offset, limit)
	pageJSONResponse(w, page, total, offset, limit)
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
	CounterScope    string   `json:"counterScope"`
	CounterInterval *int     `json:"counterInterval"`
	SessionMode     string   `json:"sessionMode"`
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
	req.CounterScope = strings.TrimSpace(req.CounterScope)
	req.SessionMode = strings.TrimSpace(req.SessionMode)
	req.TargetAgentID = strings.TrimSpace(req.TargetAgentID)
	req.FlowID = strings.TrimSpace(req.FlowID)
	if strings.TrimSpace(req.PromptTemplate) == "" {
		writeError(w, http.StatusBadRequest, "promptTemplate is required")
		return
	}
	// Pointer→value extraction for the interval fields, and the one check the shape
	// validator cannot express: "you omitted a required interval" (an absent field
	// reads as 0, which db.ValidateAutomationShape would otherwise report as a range
	// error). Every other per-kind rule — boardOp/action, scope, threshold/interval
	// bounds, tag presence, target presence — is enforced ONCE by ValidateAutomationShape
	// below, so it cannot drift from the agent-tool path.
	tokenThreshold := 0
	if req.TriggerKind == db.TriggerToken {
		if req.TokenThreshold == nil {
			writeError(w, http.StatusBadRequest, "tokenThreshold is required for token automations")
			return
		}
		tokenThreshold = *req.TokenThreshold
	}
	counterInterval := 0
	if req.TriggerKind == db.TriggerCounter {
		if req.CounterInterval == nil {
			writeError(w, http.StatusBadRequest, "counterInterval is required for counter automations")
			return
		}
		counterInterval = *req.CounterInterval
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
		CounterScope:    req.CounterScope,
		CounterInterval: counterInterval,
		SessionMode:     req.SessionMode,
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
	// TriggerKind is applied only when the request specifies it (a partial patch
	// like spawnTags-only sends "" and must not flip the kind). A full edit sends
	// the kind, so the kind-specific filters are (re)applied together with it. All
	// format/range/coherence checks on the MERGED result are enforced once by
	// db.ValidateAutomationShape below — the same validator the create + agent-tool
	// paths run, so the entry points cannot drift.
	if k := strings.TrimSpace(req.TriggerKind); k != "" {
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
			cur.CounterScope = strings.TrimSpace(req.CounterScope)
		}
	}
	// Interval fields are pointers: absent in a partial patch means "leave as
	// stored"; ValidateAutomationShape validates whatever the merge ends up with.
	if req.TokenThreshold != nil {
		cur.TokenThreshold = *req.TokenThreshold
	}
	if req.CounterInterval != nil {
		cur.CounterInterval = *req.CounterInterval
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
	// Name, spawnTags and sessionMode are always taken from the request (they may be
	// cleared; an empty sessionMode falls back to the per-kind default).
	cur.Name = strings.TrimSpace(req.Name)
	cur.SpawnTags = req.SpawnTags
	cur.SessionMode = strings.TrimSpace(req.SessionMode)
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

// handleGenerateAutomationTitle asks the runtime's title model for a short name
// and SUGGESTS it — it does not write. The source is built from the automation's
// trigger kind, target and prompt template.
//
// Suggest-only is deliberate: the button lives inside an edit modal, so writing
// here would persist a name the user never confirmed (and could not undo by
// pressing Cancel), while leaving the caller's list showing the old one. The
// name travels with the modal's normal save instead.
func (s *Server) handleGenerateAutomationTitle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	if wsp.Runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime not available")
		return
	}
	auto, err := wsp.DB.GetAutomation(r.Context(), id)
	if writeDBError(w, err, "automation not found") {
		return
	}
	kind := auto.TriggerKind
	if kind == "" {
		kind = db.TriggerTag
	}
	source := fmt.Sprintf("Automation: trigger=%s", kind)
	if auto.Name != "" {
		source += fmt.Sprintf(" currentName=%s", auto.Name)
	}
	if auto.TargetAgentID != "" {
		if ag, err := wsp.DB.GetAgent(r.Context(), auto.TargetAgentID); err == nil {
			source += fmt.Sprintf(" agent=%s", ag.Name)
		}
	}
	if auto.FlowID != "" {
		if fl, err := wsp.DB.GetFlow(r.Context(), auto.FlowID); err == nil {
			source += fmt.Sprintf(" flow=%s", fl.Name)
		}
	}
	if auto.PromptTemplate != "" {
		source += fmt.Sprintf(" prompt=%s", auto.PromptTemplate)
	}
	// TitleFor DEGRADES rather than returning empty: on any failure it hands back
	// FallbackTitle(source), which here would be the raw "Automation: trigger=… prompt=…"
	// string. Persisting that would silently overwrite the name with the prompt, so
	// the error has to stop the write, not just get logged.
	title, err := wsp.Runtime.TitleFor(r.Context(), "", source)
	if err != nil {
		s.logger.Warn("automation title generation failed", "id", id, "error", err)
		writeError(w, http.StatusBadGateway, "title generation failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"title": title})
}
