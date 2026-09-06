package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/logbuf"
	"github.com/bilal-arikan/tionharness/internal/textutil"
)

// handleListLogs returns recent captured log entries (application + all
// workspaces). Query params: limit (default 500), level (minimum level filter:
// debug|info|warn|error), q (case-insensitive substring over message + attrs),
// component (exact match on the originating subsystem), session (exact match on
// the session id), since / until (unix milliseconds, inclusive time window).
func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	limit := 500
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	minLevel := levelRank(r.URL.Query().Get("level"))
	q := textutil.FoldLower(strings.TrimSpace(r.URL.Query().Get("q")))
	component := strings.TrimSpace(r.URL.Query().Get("component"))
	session := strings.TrimSpace(r.URL.Query().Get("session"))
	parseMs := func(key string) int64 {
		if v := r.URL.Query().Get(key); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				return n
			}
		}
		return 0
	}
	since, until := parseMs("since"), parseMs("until")

	// Newest-first walk that stops at `limit` matches: the ring is never copied
	// whole, and the text match only runs on rows the cheaper facets kept.
	filtered := s.logs.Collect(limit, func(e logbuf.Entry) bool {
		if levelRank(e.Level) < minLevel {
			return false
		}
		if component != "" && e.Component != component {
			return false
		}
		if session != "" && e.Session != session {
			return false
		}
		if since > 0 && e.Time < since {
			return false
		}
		if until > 0 && e.Time > until {
			return false
		}
		return q == "" || entryMatches(e, q)
	})

	if filtered == nil {
		filtered = []logbuf.Entry{}
	}
	writeJSON(w, http.StatusOK, filtered)
}

// clientLogReport is a single error/diagnostic forwarded from the frontend
// (ErrorBoundary, window.onerror, unhandledrejection). Bridging these into the
// same slog stream means a white-screen React crash or an unhandled promise
// rejection shows up in the Logs screen instead of staying in the browser
// console where nobody is watching.
type clientLogReport struct {
	Level   string `json:"level"`   // "error" (default) | "warn" | "info"
	Source  string `json:"source"`  // e.g. "error-boundary" | "window.onerror" | "unhandledrejection"
	Message string `json:"message"` // error message / description
	Stack   string `json:"stack"`   // optional stack trace / component stack
	URL     string `json:"url"`     // page URL where it happened
}

// handleClientLog records a frontend-reported error into the app log stream. It
// is intentionally lenient: a malformed body is dropped (HTTP 204) rather than
// erroring, so the reporter never needs error handling of its own.
func (s *Server) handleClientLog(w http.ResponseWriter, r *http.Request) {
	var rep clientLogReport
	if err := decodeJSON(r, &rep); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	msg := strings.TrimSpace(rep.Message)
	if msg == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Cap attribute sizes so a runaway client payload can't dominate the buffer.
	level := slog.LevelError
	switch strings.ToLower(strings.TrimSpace(rep.Level)) {
	case "warn", "warning":
		level = slog.LevelWarn
	case "info":
		level = slog.LevelInfo
	}
	source := rep.Source
	if source == "" {
		source = "client"
	}
	s.logger.LogAttrs(r.Context(), level, "client error",
		slog.String("source", capRune(source, 64)),
		slog.String("message", capRune(msg, 1000)),
		slog.String("url", capRune(rep.URL, 300)),
		slog.String("stack", capRune(rep.Stack, 4000)),
	)
	w.WriteHeader(http.StatusNoContent)
}

// capRune truncates s to at most max runes (UTF-8 safe), appending an ellipsis
// when cut, so Turkish characters are never split mid-rune.
func capRune(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
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

// entryMatches is the free-text facet. q must be folded with
// textutil.FoldLower; the entry's fields are folded in place while scanning,
// so a query over the whole ring never allocates a lower-cased copy per row.
func entryMatches(e logbuf.Entry, q string) bool {
	if textutil.ContainsFold(e.Message, q) {
		return true
	}
	// Promoted source fields participate in free-text search like any attr.
	for _, v := range []string{e.Component, e.Session, e.Agent, e.Workspace} {
		if v != "" && textutil.ContainsFold(v, q) {
			return true
		}
	}
	for k, v := range e.Attrs {
		if textutil.ContainsFold(k, q) || textutil.ContainsFold(v, q) {
			return true
		}
	}
	return false
}
