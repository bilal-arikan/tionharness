package api

import (
	"net/http"

	"github.com/bilal/swarmgo/internal/db"
)

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agentId")
	sessions, err := ws(r).DB.ListSessions(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sessions == nil {
		sessions = []db.Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

type createSessionReq struct {
	AgentID string `json:"agentId"`
	Title   string `json:"title"`
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agentId is required")
		return
	}
	if _, err := ws(r).DB.GetAgent(r.Context(), req.AgentID); err != nil {
		writeError(w, http.StatusBadRequest, "agent not found")
		return
	}

	session, err := ws(r).DB.CreateSession(r.Context(), db.Session{
		AgentID: req.AgentID,
		Title:   req.Title,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	messages, err := ws(r).DB.ListMessages(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if messages == nil {
		messages = []db.Message{}
	}
	writeJSON(w, http.StatusOK, messages)
}
