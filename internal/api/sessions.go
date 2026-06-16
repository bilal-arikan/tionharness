package api

import (
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
)

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agentId")
	sessions, err := ws(r).DB.ListSessions(r.Context(), agentID)
	if writeDBError(w, err, "") {
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
	// agentId is the session's default agent. Optional: when omitted we fall
	// back to the workspace's first (newest) agent so a session can be created
	// session-first, then routed per-turn via "@mention".
	if req.AgentID == "" {
		agents, _ := ws(r).DB.ListAgents(r.Context())
		if len(agents) == 0 {
			writeError(w, http.StatusBadRequest, "no agents exist; create an agent first")
			return
		}
		req.AgentID = agents[0].ID
	} else if _, err := ws(r).DB.GetAgent(r.Context(), req.AgentID); err != nil {
		writeError(w, http.StatusBadRequest, "agent not found")
		return
	}

	session, err := ws(r).DB.CreateSession(r.Context(), db.Session{
		AgentID: req.AgentID,
		Title:   req.Title,
	})
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

type titleResp struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type generateTitleReq struct {
	// Source optionally overrides what the title is generated from. When empty,
	// the session's existing conversation is used.
	Source string `json:"source"`
}

// handleGenerateSessionTitle (re)generates a title for a chat session and
// persists it. The source is an explicit override or, by default, the opening
// of the session's conversation.
func (s *Server) handleGenerateSessionTitle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()

	session, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}

	var req generateTitleReq
	_ = decodeJSON(r, &req) // body is optional

	source := strings.TrimSpace(req.Source)
	if source == "" {
		msgs, err := wsp.DB.ListMessages(ctx, id)
		if writeDBError(w, err, "") {
			return
		}
		source = titleSourceFromMessages(msgs)
	}
	if source == "" {
		writeError(w, http.StatusBadRequest, "no content to generate a title from")
		return
	}

	title, err := wsp.Runtime.TitleFor(ctx, session.AgentID, source)
	if err != nil {
		s.logger.Warn("session title generation degraded", "session", id, "error", err)
	}
	if err := wsp.DB.SetSessionTitle(ctx, id, title); writeDBError(w, err, "session not found") {
		return
	}
	writeJSON(w, http.StatusOK, titleResp{ID: id, Title: title})
}

// titleSourceFromMessages composes a compact source string from the first few
// turns of a conversation for titling.
func titleSourceFromMessages(msgs []db.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteString("\n")
		if b.Len() > 1500 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	messages, err := ws(r).DB.ListMessages(r.Context(), sessionID)
	if writeDBError(w, err, "") {
		return
	}
	if messages == nil {
		messages = []db.Message{}
	}
	writeJSON(w, http.StatusOK, messages)
}
