package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/bilal/swarmgo/internal/logbuf"
)

// handleListLogs returns recent captured log entries (application + all
// workspaces). Query params: limit (default 500), level (minimum level filter:
// debug|info|warn|error), q (case-insensitive substring over message + attrs).
func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	limit := 500
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	entries := s.logs.Entries(0) // all retained; filter then trim below
	minLevel := levelRank(r.URL.Query().Get("level"))
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	filtered := make([]logbuf.Entry, 0, len(entries))
	for _, e := range entries {
		if levelRank(e.Level) < minLevel {
			continue
		}
		if q != "" && !entryMatches(e, q) {
			continue
		}
		filtered = append(filtered, e)
	}

	// Keep the most recent `limit` after filtering.
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	writeJSON(w, http.StatusOK, filtered)
}

// levelRank maps a slog level name to an ordered rank for min-level filtering.
// An empty/unknown value ranks below DEBUG so it never filters anything out.
func levelRank(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return 1
	case "INFO":
		return 2
	case "WARN", "WARNING":
		return 3
	case "ERROR":
		return 4
	default:
		return 0
	}
}

func entryMatches(e logbuf.Entry, q string) bool {
	if strings.Contains(strings.ToLower(e.Message), q) {
		return true
	}
	for k, v := range e.Attrs {
		if strings.Contains(strings.ToLower(k), q) || strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}
