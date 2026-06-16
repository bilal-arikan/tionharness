package api

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder wraps http.ResponseWriter to capture the response status code
// for access logging. It re-exposes http.Flusher so SSE streaming endpoints
// (chat stream, event feed) keep working when wrapped — they type-assert the
// writer to http.Flusher, which would otherwise fail through the wrapper.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status = http.StatusOK
		r.wrote = true
	}
	return r.ResponseWriter.Write(b)
}

// Flush forwards to the underlying writer so SSE endpoints keep streaming.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// withRequestLog logs every API request (method, path, status, duration) so
// normal activity is visible in the in-app Logs screen — previously only errors
// and autonomous events produced log records, so an actively-used app showed no
// new logs. High-frequency or long-lived endpoints (the logs poll itself, the
// SSE event feed, health checks) are skipped to keep the ring buffer signal-rich.
func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skipRequestLog(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		level := slog.LevelInfo
		switch {
		case rec.status >= 500:
			level = slog.LevelError
		case rec.status >= 400:
			level = slog.LevelWarn
		}
		s.logger.LogAttrs(r.Context(), level, "http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.String("dur", time.Since(start).Round(time.Millisecond).String()),
		)
	})
}

// skipRequestLog reports paths that should not be access-logged: the logs poll
// (would flood the buffer it feeds), the long-lived SSE event feed, and health.
func skipRequestLog(path string) bool {
	switch path {
	case "/api/logs", "/api/events", "/health":
		return true
	}
	return false
}
