package api

import (
	"errors"
	"net/http"

	"github.com/bilal/swarmgo/internal/db"
)

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := ws(r).DB.ListSchedules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := wsp.Scheduler.Reload(r.Context()); err != nil {
		s.logger.Warn("scheduler reload failed", "error", err)
	}
	writeJSON(w, http.StatusCreated, schedule)
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
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := wsp.Scheduler.Reload(r.Context()); err != nil {
		s.logger.Warn("scheduler reload failed", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": req.Enabled})
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)

	err := wsp.DB.DeleteSchedule(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := wsp.Scheduler.Reload(r.Context()); err != nil {
		s.logger.Warn("scheduler reload failed", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "result": "deleted"})
}
