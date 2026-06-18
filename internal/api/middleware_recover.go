package api

import (
	"net/http"
	"runtime/debug"
)

// withRecover is the outermost safety net for HTTP handlers: it recovers any
// panic that escapes a handler, records it in the app log stream (so it shows in
// the Logs screen and not just on stderr, where net/http's default recovery
// drops it), and returns a clean 500 instead of a dropped/half-written response.
//
// http.ErrAbortHandler is re-panicked so the server can still abort a streaming
// response on purpose; everything else is treated as an unexpected crash.
func (s *Server) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec) // intentional stream abort — let net/http handle it
			}
			s.logger.Error("http handler panicked",
				"method", r.Method, "path", r.URL.Path,
				"panic", rec, "stack", string(debug.Stack()))
			// Best-effort clean 500. If the handler already wrote headers (e.g. it
			// panicked mid-SSE-stream), WriteHeader is a no-op and a final Write may
			// fail on a hijacked/closed conn — swallow that so recovery never panics.
			defer func() { _ = recover() }()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal server error"}`))
		}()
		next.ServeHTTP(w, r)
	})
}
