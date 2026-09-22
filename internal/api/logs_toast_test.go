package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/logbuf"
)

// newLogServer wires a Server whose logger captures into a real ring buffer,
// with the same "component=api" base attr the app installs, so the tests cover
// the component override end to end.
func newLogServer() *Server {
	buf := logbuf.New(100)
	logger := slog.New(buf.Handler(slog.NewTextHandler(io.Discard, nil))).With("component", "api")
	return &Server{logger: logger, logs: buf}
}

func postClientLog(t *testing.T, s *Server, body string) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleClientLog(rec, httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(body)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d, want 204", rec.Code)
	}
}

func listLogs(t *testing.T, s *Server, query string) []logbuf.Entry {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleListLogs(rec, httptest.NewRequest(http.MethodGet, "/api/logs?"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	var out []logbuf.Entry
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// TestHandleClientLog_Toast stores a toast report under component ui-toast
// with its detail, stack and client time, retrievable via the list filter.
func TestHandleClientLog_Toast(t *testing.T) {
	s := newLogServer()
	postClientLog(t, s, `{"level":"warn","source":"toast","message":"save failed","detail":"HTTP 500","stack":"at save()","url":"http://x/y","time":1700000000000}`)
	postClientLog(t, s, `{"level":"error","source":"window.onerror","message":"boom"}`)

	got := listLogs(t, s, "component=ui-toast")
	if len(got) != 1 {
		t.Fatalf("ui-toast entries = %d, want 1: %+v", len(got), got)
	}
	e := got[0]
	if e.Level != "WARN" {
		t.Errorf("level = %q, want WARN", e.Level)
	}
	if e.Message != "ui toast: save failed" {
		t.Errorf("message = %q", e.Message)
	}
	for k, want := range map[string]string{
		"source": "toast", "message": "save failed", "detail": "HTTP 500",
		"stack": "at save()", "url": "http://x/y", "client_time": "1700000000000",
	} {
		if e.Attrs[k] != want {
			t.Errorf("attr %s = %q, want %q", k, e.Attrs[k], want)
		}
	}

	// Non-toast client reports land under the generic ui component.
	ui := listLogs(t, s, "component=ui")
	if len(ui) != 1 || ui[0].Message != "client error" {
		t.Fatalf("ui entries = %+v, want one 'client error'", ui)
	}

	// Level facet: a min-level of error excludes the warn toast.
	if errs := listLogs(t, s, "component=ui-toast&level=error"); len(errs) != 0 {
		t.Errorf("level=error returned %d toast entries, want 0", len(errs))
	}
}
