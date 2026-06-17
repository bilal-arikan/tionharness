package api

import (
	"net/http"

	"github.com/bilal/swarmgo/internal/db"
)

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := ws(r).DB.ListSchedules(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	if schedules == nil {
		schedules = []db.Schedule{}
	}
	writeJSON(w, http.StatusOK, schedules)
}

type createScheduleReq struct {
	AgentID  string `json:"agentId"`
	TaskID   string `json:"taskId"`
	CronExpr string `json:"cronExpr"`
	Prompt   string `json:"prompt"`
	Enabled  bool   `json:"enabled"`
}

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)

	var req createScheduleReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agentId is required")
		return
	}
	if req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "cronExpr is required")
		return
	}
	if req.TaskID == "" && req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "either taskId or prompt is required")
		return
	}
	if _, err := wsp.DB.GetAgent(r.Context(), req.AgentID); err != nil {
		writeError(w, http.StatusBadRequest, "unknown agent")
		return
	}

	schedule, err := wsp.DB.CreateSchedule(r.Context(), db.Schedule{
		AgentID:  req.AgentID,
		TaskID:   req.TaskID,
		CronExpr: req.CronExpr,
		Prompt:   req.Prompt,
		Enabled:  req.Enabled,
	})
	if writeDBError(w, err, "") {
		return
	}
	if err := wsp.Scheduler.Reload(r.Context()); err != nil {
		s.logger.Warn("scheduler reload failed", "error", err)
	}
	s.logger.Info("schedule created", "id", schedule.ID, "agent", req.AgentID, "cron", req.CronExpr)
	writeJSON(w, http.StatusCreated, schedule)
}

type updateScheduleReq struct {
	AgentID  string `json:"agentId"`
	TaskID   string `json:"taskId"`
	CronExpr string `json:"cronExpr"`
	Prompt   string `json:"prompt"`
}

// handleUpdateSchedule edits a schedule's agent/cron/task/prompt and reloads cron.
func (s *Server) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)

	var req updateScheduleReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agentId is required")
		return
	}
	if req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "cronExpr is required")
		return
	}
	if req.TaskID == "" && req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "either taskId or prompt is required")
		return
	}
	if _, err := wsp.DB.GetAgent(r.Context(), req.AgentID); err != nil {
		writeError(w, http.StatusBadRequest, "unknown agent")
		return
	}

	err := wsp.DB.UpdateSchedule(r.Context(), db.Schedule{
		ID:       id,
		AgentID:  req.AgentID,
		CronExpr: req.CronExpr,
		TaskID:   req.TaskID,
		Prompt:   req.Prompt,
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

	var req toggleScheduleReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
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
	runErr := wsp.Scheduler.RunNow(r.Context(), id)
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
