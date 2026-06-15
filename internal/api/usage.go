package api

import (
	"errors"
	"net/http"

	"github.com/bilal/swarmgo/internal/conversation"
	"github.com/bilal/swarmgo/internal/db"
)

// handleAgentUsage returns today's usage plus the agent's daily caps, so the UI
// can render a "X / limit" spend meter.
func (s *Server) handleAgentUsage(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	wsp := ws(r)

	agent, err := wsp.DB.GetAgent(r.Context(), agentID)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	usage, err := wsp.DB.GetUsageToday(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"day":             usage.Day,
		"calls":           usage.Calls,
		"inputTokens":     usage.InputTokens,
		"outputTokens":    usage.OutputTokens,
		"dailyCallLimit":  agent.DailyCallLimit,
		"dailyTokenLimit": agent.DailyTokenLimit,
	})
}

type setBudgetReq struct {
	DailyCallLimit  int `json:"dailyCallLimit"`
	DailyTokenLimit int `json:"dailyTokenLimit"`
}

// handleSetBudget updates an agent's daily spend caps (0 = unlimited).
func (s *Server) handleSetBudget(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	wsp := ws(r)

	var req setBudgetReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.DailyCallLimit < 0 || req.DailyTokenLimit < 0 {
		writeError(w, http.StatusBadRequest, "limits must be >= 0")
		return
	}
	err := wsp.DB.UpdateBudget(r.Context(), agentID, req.DailyCallLimit, req.DailyTokenLimit)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"agentId":         agentID,
		"dailyCallLimit":  req.DailyCallLimit,
		"dailyTokenLimit": req.DailyTokenLimit,
	})
}

// handleSessionContext reports a session's estimated context size and summary
// state — the data behind the chat context meter.
func (s *Server) handleSessionContext(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	wsp := ws(r)

	session, err := wsp.DB.GetSession(r.Context(), sessionID)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	history, err := wsp.DB.ListMessages(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pending := history
	if session.SummaryMsgCount <= len(history) {
		pending = history[session.SummaryMsgCount:]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"contextTokens":   conversation.EstimateTokens(session.Summary, pending),
		"hasSummary":      session.Summary != "",
		"summaryMsgCount": session.SummaryMsgCount,
		"messageCount":    session.MessageCount,
	})
}
