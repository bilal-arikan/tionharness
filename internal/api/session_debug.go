package api

// handleSessionDebug exposes a session's per-session debug journal — the parallel
// observability stream (turn timings, per-call token spend, tool latency/size,
// hook decisions, errors, compaction, recovery) the runtime appends to
// debug.jsonl. It powers the Debug tab in the session detail panel and mirrors
// the read_session_debug agent tool.
//
//   GET /api/sessions/{id}/debug                -> { summary: DebugSummary }
//   GET /api/sessions/{id}/debug?summary=0      -> { events: []DebugEvent }
//   GET /api/sessions/{id}/debug?summary=0&type=tool&limit=200
//
// Default returns the aggregate summary; summary=0 returns the raw event list
// (newest last), optionally filtered by type and capped by limit.

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleSessionDebug(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	wsp := ws(r)
	q := r.URL.Query()

	// Default to the summary view; summary=0/false switches to the raw events list.
	if sv := q.Get("summary"); sv == "0" || sv == "false" {
		limit := 200
		if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
			limit = l
		}
		if limit > 5000 {
			limit = 5000
		}
		evs, err := wsp.DB.ReadDebugEvents(r.Context(), sessionID, q.Get("type"), limit)
		if writeDBError(w, err, "") {
			return
		}
		if evs == nil {
			evs = nil // keep null → encoded as [] below via the map value
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"sessionId": sessionID,
			"events":    evs,
		})
		return
	}

	sum, err := wsp.DB.GetDebugSummary(r.Context(), sessionID)
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": sum})
}

// handleSessionTurnDebug returns the per-MESSAGE debug rollup for one assistant
// reply — the data behind the chat message debug button. Events are correlated by
// the reply message id (DebugEvent.TurnID). Cost is computed with the SAME pricing
// helper as the Budget screen so per-message figures reconcile with the session
// total.
//
//	GET /api/sessions/{id}/turn-debug?turn={replyMessageId}
func (s *Server) handleSessionTurnDebug(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	turnID := strings.TrimSpace(r.URL.Query().Get("turn"))
	wsp := ws(r)

	td, err := wsp.DB.GetTurnDebug(r.Context(), sessionID, turnID)
	if writeDBError(w, err, "") {
		return
	}
	// Reuse the shared Motor-B pricing so this matches /usage-detail exactly.
	_, cost, savings, priced, estimated, _, _ := modelRowsFor(td.ByModel)

	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId":        td.SessionID,
		"turnId":           td.TurnID,
		"found":            td.Found,
		"model":            td.Model,
		"durMs":            td.DurMs,
		"stop":             td.Stop,
		"llmCalls":         td.LLMCalls,
		"inputTokens":      td.InputTokens,
		"outputTokens":     td.OutputTokens,
		"cacheReadTokens":  td.CacheRead,
		"cacheWriteTokens": td.CacheWrite,
		"toolCalls":        td.ToolCalls,
		"tools":            td.Tools,
		"errors":           td.Errors,
		"recoveries":       td.Recoveries,
		"compactions":      td.Compactions,
		"lastError":        td.LastError,
		"costUSD":          cost,
		"savingsUSD":       savings,
		"priced":           priced,
		"estimated":        estimated,
		"firstTs":          td.FirstTs,
		"lastTs":           td.LastTs,
	})
}
