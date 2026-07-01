package api

import (
	"net/http"

	"github.com/bilal-arikan/swarmgo/internal/conversation"
)

// handleAgentUsage returns today's usage for an agent, so the UI can render a
// spend meter (calls + cost).
func (s *Server) handleAgentUsage(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	wsp := ws(r)

	if _, err := wsp.DB.GetAgent(r.Context(), agentID); writeDBError(w, err, "agent not found") {
		return
	}

	usage, err := wsp.DB.GetUsageToday(r.Context(), agentID)
	if writeDBError(w, err, "") {
		return
	}

	// Cost + per-model detail come from the same Motor-B helpers the Budget
	// screen uses, so the chat meters and session detail panel show figures
	// consistent with /api/usage (this is the agent's whole-day spend, not a
	// single session's).
	models, cost, savings, priced, estimated, cacheRead, cacheWrite := modelRowsFor(usage.ByModel)

	writeJSON(w, http.StatusOK, map[string]any{
		"day":                  usage.Day,
		"calls":                usage.Calls,
		"inputTokens":          usage.InputTokens,
		"outputTokens":         usage.OutputTokens,
		"cacheReadTokens":      cacheRead,
		"cacheWriteTokens":     cacheWrite,
		"byKind":               usage.ByKind, // per-origin breakdown (chat/task/schedule/flow/compact/…)
		"byModel":              models,       // per-model detail with cost
		"costUSD":              cost,
		"savingsUSD":           savings,
		"priced":               priced,
		"estimated":            estimated,
		"compactSavedBytes":    usage.CompactSavedBytes,
		"compactSavedBytesLLM": usage.CompactSavedBytesLLM,
	})
}

// handleSessionContext reports a session's estimated context size and summary
// state — the data behind the chat context meter.
func (s *Server) handleSessionContext(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	wsp := ws(r)

	session, err := wsp.DB.GetSession(r.Context(), sessionID)
	if writeDBError(w, err, "session not found") {
		return
	}
	history, err := wsp.DB.ListMessages(r.Context(), sessionID)
	if writeDBError(w, err, "") {
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
