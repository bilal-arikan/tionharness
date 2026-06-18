package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// capHandler retains records so a test can assert on what was logged.
type capHandler struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (h *capHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *capHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, r)
	return nil
}
func (h *capHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capHandler) WithGroup(string) slog.Handler      { return h }
func (h *capHandler) find(level slog.Level, msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.recs {
		if r.Level == level && r.Message == msg {
			return true
		}
	}
	return false
}

// TestWithRecover_CatchesPanic verifies a panicking handler yields a clean 500
// and an Error log record instead of crashing the server / dropping the conn.
func TestWithRecover_CatchesPanic(t *testing.T) {
	cap := &capHandler{}
	s := &Server{logger: slog.New(cap)}

	h := s.withRecover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/anything", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if !cap.find(slog.LevelError, "http handler panicked") {
		t.Error("expected an Error 'http handler panicked' log record")
	}
}

// TestWithRecover_PassesThrough verifies a normal handler is untouched.
func TestWithRecover_PassesThrough(t *testing.T) {
	s := &Server{logger: slog.New(&capHandler{})}
	h := s.withRecover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want 418 (passed through)", rec.Code)
	}
}

// TestHandleClientLog records a frontend-reported error as a log entry.
func TestHandleClientLog(t *testing.T) {
	cap := &capHandler{}
	s := &Server{logger: slog.New(cap)}

	body := `{"level":"error","source":"error-boundary","message":"render crashed","stack":"at X","url":"http://x/y"}`
	rec := httptest.NewRecorder()
	s.handleClientLog(rec, httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body)))

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if !cap.find(slog.LevelError, "client error") {
		t.Error("expected an Error 'client error' log record")
	}
}

// TestHandleClientLog_DropsEmpty verifies a malformed/empty body is dropped
// quietly (204) without logging, so the reporter needs no error handling.
func TestHandleClientLog_DropsEmpty(t *testing.T) {
	cap := &capHandler{}
	s := &Server{logger: slog.New(cap)}

	rec := httptest.NewRecorder()
	s.handleClientLog(rec, httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(`{"message":""}`)))
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if cap.find(slog.LevelError, "client error") {
		t.Error("empty message should not be logged")
	}
}
