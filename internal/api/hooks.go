package api

import (
	"net/http"

	"github.com/bilal/swarmgo/internal/db"
)

func (s *Server) handleListHooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := ws(r).DB.ListHooks(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	if hooks == nil {
		hooks = []db.Hook{}
	}
	writeJSON(w, http.StatusOK, hooks)
}

type hookReq struct {
	Event      string `json:"event"`
	Matcher    string `json:"matcher"`
	Type       string `json:"type"`
	Command    string `json:"command"`
	TimeoutSec int    `json:"timeoutSec"`
	Enabled    bool   `json:"enabled"`
}

// validHookEvent reports whether e is a supported hook event.
func validHookEvent(e string) bool {
	return e == db.HookPreToolUse || e == db.HookPostToolUse
}

func (s *Server) handleCreateHook(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	var req hookReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if !validHookEvent(req.Event) {
		writeError(w, http.StatusBadRequest, "event must be PreToolUse or PostToolUse")
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	hook, err := wsp.DB.CreateHook(r.Context(), db.Hook{
		Event:      req.Event,
		Matcher:    req.Matcher,
		Type:       "command",
		Command:    req.Command,
		TimeoutSec: req.TimeoutSec,
		Enabled:    req.Enabled,
	})
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("hook created", "id", hook.ID, "event", hook.Event, "matcher", hook.Matcher)
	writeJSON(w, http.StatusCreated, hook)
}

func (s *Server) handleUpdateHook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	var req hookReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if !validHookEvent(req.Event) {
		writeError(w, http.StatusBadRequest, "event must be PreToolUse or PostToolUse")
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	err := wsp.DB.UpdateHook(r.Context(), db.Hook{
		ID:         id,
		Event:      req.Event,
		Matcher:    req.Matcher,
		Type:       "command",
		Command:    req.Command,
		TimeoutSec: req.TimeoutSec,
		Enabled:    req.Enabled,
	})
	if writeDBError(w, err, "hook not found") {
		return
	}
	h, err := wsp.DB.GetHook(r.Context(), id)
	if writeDBError(w, err, "hook not found") {
		return
	}
	s.logger.Info("hook updated", "id", id, "event", h.Event)
	writeJSON(w, http.StatusOK, h)
}

type toggleHookReq struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleToggleHook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	var req toggleHookReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := wsp.DB.SetHookEnabled(r.Context(), id, req.Enabled); writeDBError(w, err, "hook not found") {
		return
	}
	s.logger.Info("hook toggled", "id", id, "enabled", req.Enabled)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": req.Enabled})
}

func (s *Server) handleDeleteHook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	if err := wsp.DB.DeleteHook(r.Context(), id); writeDBError(w, err, "hook not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "result": "deleted"})
}
