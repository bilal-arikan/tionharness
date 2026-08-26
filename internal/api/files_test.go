package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveFile issues GET /api/files?rel=<rel> against the workspace and returns the
// recorder.
func serveFile(t *testing.T, s *Server, wsID, rel string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/files?rel="+url.QueryEscape(rel), nil)
	req.Header.Set("X-Workspace-Id", wsID)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

// TestServeFileRelTextArtifact: a `file` artifact stored in the sandbox (e.g. a
// markdown report written by create_artifact) must be readable through
// /api/files?rel=..., otherwise the artifacts screen can neither preview nor
// download it. Text is served as text/plain — never text/html.
func TestServeFileRelTextArtifact(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	rel := "artifacts/SES1/media-1.md"
	abs := filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("# report"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec := serveFile(t, s, wsp.ID, rel)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header on text response")
	}
	if rec.Body.String() != "# report" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

// TestServeFileAbsoluteTextRejected: the text fallback is limited to the sandbox
// branch — an absolute path to a text file stays unsupported so /api/files cannot
// be used to read arbitrary files off disk.
func TestServeFileAbsoluteTextRejected(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	abs := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(abs, []byte("nope"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/files?path="+url.QueryEscape(abs), nil)
	req.Header.Set("X-Workspace-Id", wsp.ID)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415, body = %s", rec.Code, rec.Body.String())
	}
}

// TestServeFileRelUnknownExtRejected: extensions outside both allowlists stay
// rejected even inside the sandbox.
func TestServeFileRelUnknownExtRejected(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	rel := "artifacts/SES1/blob.bin"
	abs := filepath.Join(wsp.SandboxRoot(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if rec := serveFile(t, s, wsp.ID, rel); rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415, body = %s", rec.Code, rec.Body.String())
	}
}

// TestServeFileRelativePathParam: a chat reply may embed a workspace-relative
// image path (![shot](output/images/a.png)). The markdown renderer turns that
// into /api/files?path=output/images/a.png, so a relative `path` must resolve
// against the workspace sandbox instead of the server's working directory.
func TestServeFileRelativePathParam(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	abs := filepath.Join(wsp.SandboxRoot(), "output", "images", "a.png")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("\x89PNG\r\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/files?path="+url.QueryEscape("output/images/a.png"), nil)
	req.Header.Set("X-Workspace-Id", wsp.ID)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q, want image/png", ct)
	}
}

// TestServeFileRelativeTraversalRejected: the relative-path fallback must not
// become an escape hatch out of the sandbox.
func TestServeFileRelativeTraversalRejected(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/files?path="+url.QueryEscape("../../secret.png"), nil)
	req.Header.Set("X-Workspace-Id", wsp.ID)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}
