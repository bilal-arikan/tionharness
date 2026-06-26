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
