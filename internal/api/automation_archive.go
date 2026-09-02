package api

import (
	"net/http"
	"strconv"
)

// Archive endpoints (_Docs/77 R5). Archiving is the curator's ceiling on
// destructive action: the rule/schedule/hook keeps its configuration and
// history, stops firing, leaves the default lists, and can be restored. It is
// deliberately separate from Enabled (the user's on/off switch) so a restore
// brings the entity back exactly as it was.

type archiveReq struct {
	Archived bool `json:"archived"`
}

func (s *Server) handleArchiveAutomation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[archiveReq](w, r)
	if !ok {
		return
	}
	if err := ws(r).DB.SetAutomationArchived(r.Context(), id, req.Archived); writeDBError(w, err, "automation not found") {
		return
	}
	s.logger.Info("automation archived", "id", id, "archived", req.Archived)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "archived": req.Archived})
}

// handleAutomationFires serves the rule's fire ledger (fired / skipped / failed
// attempts with reasons), newest first. ?limit bounds the count (default 100).
func (s *Server) handleAutomationFires(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	if _, err := wsp.DB.GetAutomation(r.Context(), id); writeDBError(w, err, "automation not found") {
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	recs, err := wsp.DB.ListAutomationFires(r.Context(), id, limit)
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, recs)
}

func (s *Server) handleArchiveSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	req, ok := bindJSON[archiveReq](w, r)
	if !ok {
		return
	}
	if err := wsp.DB.SetScheduleArchived(r.Context(), id, req.Archived); writeDBError(w, err, "schedule not found") {
		return
	}
	// The cron table only holds enabled, non-archived rows: rebuild it.
	if wsp.Scheduler != nil {
		if err := wsp.Scheduler.Reload(r.Context()); err != nil {
			s.logger.Warn("scheduler reload failed", "error", err)
		}
	}
	s.logger.Info("schedule archived", "id", id, "archived", req.Archived)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "archived": req.Archived})
}

func (s *Server) handleArchiveHook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[archiveReq](w, r)
	if !ok {
		return
	}
	if err := ws(r).DB.SetHookArchived(r.Context(), id, req.Archived); writeDBError(w, err, "hook not found") {
		return
	}
	s.logger.Info("hook archived", "id", id, "archived", req.Archived)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "archived": req.Archived})
}
