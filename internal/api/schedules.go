package api

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	all, err := ws(r).DB.ListSchedules(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	// Hide one-shot wakes (schedule_wake) from the routine list: they are transient,
	// single-use timers tied to a chat turn, not user-managed recurring routines.
	schedules := make([]db.Schedule, 0, len(all))
	for _, sc := range all {
		if sc.OneShot {
			continue
		}
		schedules = append(schedules, sc)
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
	agentID := q.Get("agentId")
	matches := make([]db.Schedule, 0, len(schedules))
	for _, sc := range schedules {
		if hasEnabled && sc.Enabled != *enabled {
			continue
		}
		if agentID != "" && sc.AgentID != agentID {
			continue
		}
		matches = append(matches, sc)
	}
	if !listing {
		writeJSON(w, http.StatusOK, matches)
		return
	}
	if field != "" {
		less, err := tools.SortByField(matches, field, asc,
			func(sc db.Schedule) int64 { return sc.UpdatedAt },
			func(sc db.Schedule) int64 { return sc.CreatedAt },
			func(sc db.Schedule) string { return sc.ID }) // schedules have no name — documented surrogate
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sort.SliceStable(matches, less)
	}
	page, total := tools.SlicePage(matches, offset, limit)
	pageJSONResponse(w, page, total, offset, limit)
}

type createScheduleReq struct {
	AgentID  string `json:"agentId"`
	CronExpr string `json:"cronExpr"`
	Prompt   string `json:"prompt"`
	// FlowID, when set, makes this a flow-backed schedule (runs the flow with
	// Prompt as input instead of delivering the prompt to AgentID).
	FlowID  string `json:"flowId"`
	Enabled bool   `json:"enabled"`
	// ExpiresAt is an optional end date (unix seconds); 0 = no end date.
	ExpiresAt int64 `json:"expiresAt"`
}

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)

	req, ok := bindJSON[createScheduleReq](w, r)
	if !ok {
		return
	}
	if req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "cronExpr is required")
		return
	}
	// A schedule targets EITHER a flow or a single agent. Flow-backed schedules
	// take Prompt as the (optional) flow input; agent-backed ones require a prompt.
	if req.FlowID != "" {
		if _, err := wsp.DB.GetFlow(r.Context(), req.FlowID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown flow")
			return
		}
	} else {
		if req.AgentID == "" {
			writeError(w, http.StatusBadRequest, "agentId or flowId is required")
			return
		}
		if req.Prompt == "" {
			writeError(w, http.StatusBadRequest, "prompt is required")
			return
		}
		if _, err := wsp.DB.GetAgent(r.Context(), req.AgentID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown agent")
			return
		}
	}

	schedule, err := wsp.DB.CreateSchedule(r.Context(), db.Schedule{
		AgentID:   req.AgentID,
		CronExpr:  req.CronExpr,
		Prompt:    req.Prompt,
		FlowID:    req.FlowID,
		Enabled:   req.Enabled,
		ExpiresAt: req.ExpiresAt,
	})
	if writeDBError(w, err, "") {
		return
	}
	if err := wsp.Scheduler.Reload(r.Context()); err != nil {
		s.logger.Warn("scheduler reload failed", "error", err)
	}
	s.logger.Info("schedule created", "id", schedule.ID, "agent", req.AgentID, "flow", req.FlowID, "cron", req.CronExpr)
	writeJSON(w, http.StatusCreated, schedule)
}

type updateScheduleReq struct {
	AgentID  string `json:"agentId"`
	CronExpr string `json:"cronExpr"`
	Prompt   string `json:"prompt"`
	// FlowID, when set, makes this a flow-backed schedule (empty string clears it
	// back to agent-backed).
	FlowID string `json:"flowId"`
	// ExpiresAt is an optional end date (unix seconds); 0 = no end date.
	ExpiresAt int64 `json:"expiresAt"`
}

// handleUpdateSchedule edits a schedule's agent/flow/cron/prompt and reloads cron.
func (s *Server) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)

	req, ok := bindJSON[updateScheduleReq](w, r)
	if !ok {
		return
	}
	if req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "cronExpr is required")
		return
	}
	if req.FlowID != "" {
		if _, err := wsp.DB.GetFlow(r.Context(), req.FlowID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown flow")
			return
		}
	} else {
		if req.AgentID == "" {
			writeError(w, http.StatusBadRequest, "agentId or flowId is required")
			return
		}
		if req.Prompt == "" {
			writeError(w, http.StatusBadRequest, "prompt is required")
			return
		}
		if _, err := wsp.DB.GetAgent(r.Context(), req.AgentID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown agent")
			return
		}
	}

	err := wsp.DB.UpdateSchedule(r.Context(), db.Schedule{
		ID:        id,
		AgentID:   req.AgentID,
		CronExpr:  req.CronExpr,
		Prompt:    req.Prompt,
		FlowID:    req.FlowID,
		ExpiresAt: req.ExpiresAt,
	})
	if writeDBError(w, err, "schedule not found") {
		return
	}
	if err := wsp.Scheduler.Reload(r.Context()); err != nil {
		s.logger.Warn("scheduler reload failed", "error", err)
	}
	sc, err := wsp.DB.GetSchedule(r.Context(), id)
	if writeDBError(w, err, "schedule not found") {
		return
	}
	s.logger.Info("schedule updated", "id", id, "agent", req.AgentID, "cron", req.CronExpr)
	writeJSON(w, http.StatusOK, sc)
}

type toggleScheduleReq struct {
	Enabled bool `json:"enabled"`
}

// handleToggleSchedule enables/disables a schedule and reloads the cron table.
func (s *Server) handleToggleSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)

	req, ok := bindJSON[toggleScheduleReq](w, r)
	if !ok {
		return
	}
	err := wsp.DB.SetScheduleEnabled(r.Context(), id, req.Enabled)
	if writeDBError(w, err, "schedule not found") {
		return
	}
	if err := wsp.Scheduler.Reload(r.Context()); err != nil {
		s.logger.Warn("scheduler reload failed", "error", err)
	}
	s.logger.Info("schedule toggled", "id", id, "enabled", req.Enabled)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": req.Enabled})
}

// handleRunSchedule fires a schedule immediately ("Run" button), regardless of
// its enabled state, and returns the updated row (carrying the delivery outcome).
func (s *Server) handleRunSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)

	if _, err := wsp.DB.GetSchedule(r.Context(), id); err != nil {
		writeDBError(w, err, "schedule not found")
		return
	}
	// Run synchronously like task "run now"; the attempt is recorded on the
	// schedule even when it fails, so we always return the updated row.
	//
	// DETACH the turn from the client request: a page refresh / navigation aborts
	// this POST, and if RunNow ran on r.Context() that abort would cancel the
	// in-flight turn mid-generation — the schedule would persist a partial reply
	// and look "cut off". Mirror the chat-stream detach (context.WithoutCancel) so
	// generation runs to completion regardless of the client; a generous timeout
	// still bounds a genuinely hung run. Cron fires already detach via Background.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
	defer cancel()
	runErr := wsp.Scheduler.RunNow(runCtx, id)
	sc, err := wsp.DB.GetSchedule(r.Context(), id)
	if writeDBError(w, err, "schedule not found") {
		return
	}
	if runErr != nil {
		s.logger.Warn("schedule manual run failed", "id", id, "error", runErr)
	} else {
		s.logger.Info("schedule manual run", "id", id)
	}
	writeJSON(w, http.StatusOK, sc)
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)

	err := wsp.DB.DeleteSchedule(r.Context(), id)
	if writeDBError(w, err, "schedule not found") {
		return
	}
	if err := wsp.Scheduler.Reload(r.Context()); err != nil {
		s.logger.Warn("scheduler reload failed", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "result": "deleted"})
}
