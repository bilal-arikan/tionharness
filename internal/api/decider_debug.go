package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionharness/internal/decider"
)

func (s *Server) handleDeciderDebug(w http.ResponseWriter, r *http.Request) {
	hub := s.deciderHubOrError(w)
	if hub == nil {
		return
	}
	q := r.URL.Query()
	parse := func(key string, fallback, max int) (int, bool) {
		if q.Get(key) == "" {
			return fallback, true
		}
		v, err := strconv.Atoi(q.Get(key))
		return v, err == nil && v > 0 && v <= max
	}
	days, ok := parse("days", 7, 90)
	if !ok {
		writeError(w, http.StatusBadRequest, "days must be between 1 and 90")
		return
	}
	limit, ok := parse("limit", 500, 5000)
	if !ok {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and 5000")
		return
	}
	for _, key := range []string{"authority", "instance", "ref", "traceId"} {
		if len(q.Get(key)) > 128 {
			writeError(w, http.StatusBadRequest, "debug filter is too long")
			return
		}
	}
	writeJSON(w, http.StatusOK, hub.Debug(decider.DebugFilter{
		Since: time.Now().Add(-time.Duration(days) * 24 * time.Hour), Limit: limit,
		Authority: q.Get("authority"), Instance: q.Get("instance"), Ref: q.Get("ref"), TraceID: q.Get("traceId"),
	}))
}
