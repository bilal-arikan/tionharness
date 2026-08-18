package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// codexTestServer builds a Server whose providers.Registry has a fake codex
// binary path configured, so handleCodex* handlers see Installed()==true
// without needing a real codex on the test machine's PATH.
func codexTestServer(t *testing.T, binPath string) *Server {
	t.Helper()
	reg := providers.NewRegistry("")
	reg.SetCodexCLIPath(binPath)
	return &Server{providers: reg}
}

// withWorkspaceCtx attaches a minimal *workspace.Workspace (just DataDir, the
// only field codexHomeDirFor reads) to r's context, mirroring what the real
// workspace-resolution middleware does before a handler runs.
func withWorkspaceCtx(r *http.Request, dataDir string) *http.Request {
	ws := &workspace.Workspace{DataDir: dataDir}
	return r.WithContext(context.WithValue(r.Context(), workspaceCtxKey, ws))
}

func TestHandleWorkspaceCodexAuthNotInstalled(t *testing.T) {
	s := codexTestServer(t, "") // SetCodexCLIPath("") re-runs PATH auto-detect; assume test host has none
	if s.providers.CodexCLIPath() != "" {
		t.Skip("host has a real codex on PATH; skipping the not-installed case")
	}
	r := withWorkspaceCtx(httptest.NewRequest(http.MethodGet, "/api/workspace-settings/codex-auth", nil), t.TempDir())
	rec := httptest.NewRecorder()
	s.handleWorkspaceCodexAuth(rec, r)

	var got codexAuthDTO
	mustDecode(t, rec, &got)
	if got.Installed || got.LoggedIn {
		t.Fatalf("got %+v, want Installed=false LoggedIn=false", got)
	}
}

func TestHandleWorkspaceCodexAuthNotLoggedIn(t *testing.T) {
	s := codexTestServer(t, "fake-codex-binary")
	home := t.TempDir()
	r := withWorkspaceCtx(httptest.NewRequest(http.MethodGet, "/api/workspace-settings/codex-auth", nil), home)
	rec := httptest.NewRecorder()
	s.handleWorkspaceCodexAuth(rec, r)

	var got codexAuthDTO
	mustDecode(t, rec, &got)
	if !got.Installed || got.LoggedIn {
		t.Fatalf("got %+v, want Installed=true LoggedIn=false (no auth.json yet)", got)
	}
}

func TestHandleCodexDeviceStartMissingBinary(t *testing.T) {
	s := codexTestServer(t, "")
	if s.providers.CodexCLIPath() != "" {
		t.Skip("host has a real codex on PATH")
	}
	r := withWorkspaceCtx(httptest.NewRequest(http.MethodPost, "/api/workspace-settings/codex-auth/device/start", nil), t.TempDir())
	rec := httptest.NewRecorder()
	s.handleCodexDeviceStart(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleCodexDeviceStatusNoFlow(t *testing.T) {
	s := codexTestServer(t, "fake-codex-binary")
	r := withWorkspaceCtx(httptest.NewRequest(http.MethodGet, "/api/workspace-settings/codex-auth/device/status", nil), t.TempDir())
	rec := httptest.NewRecorder()
	s.handleCodexDeviceStatus(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when no flow has been started", rec.Code)
	}
}

func TestHandleCodexDeviceCancelIdempotentWithNoFlow(t *testing.T) {
	s := codexTestServer(t, "fake-codex-binary")
	r := withWorkspaceCtx(httptest.NewRequest(http.MethodPost, "/api/workspace-settings/codex-auth/device/cancel", nil), t.TempDir())
	rec := httptest.NewRecorder()
	s.handleCodexDeviceCancel(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (cancel with no flow is a no-op, not an error)", rec.Code)
	}
}

func TestHandleCodexAPIKeyLoginRequiresKey(t *testing.T) {
	s := codexTestServer(t, "fake-codex-binary")
	body, _ := json.Marshal(codexAPIKeyReq{APIKey: ""})
	r := withWorkspaceCtx(httptest.NewRequest(http.MethodPost, "/api/workspace-settings/codex-auth/api-key", bytes.NewReader(body)), t.TempDir())
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleCodexAPIKeyLogin(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for empty apiKey", rec.Code)
	}
}

// TestCodexHomeDirMatchesAgentResolution pins codexHomeDirFor to the exact
// same rule agent.workspaceCodexHomeDir uses (<workspace>/codex-home, a
// sibling of <workspace>/workspace) so the UI never authenticates a
// different home than the one a turn actually runs against.
func TestCodexHomeDirMatchesAgentResolution(t *testing.T) {
	r := withWorkspaceCtx(httptest.NewRequest(http.MethodGet, "/", nil), `C:\data\ws1`)
	got := codexHomeDirFor(r)
	want := `C:\data\ws1\codex-home`
	if got != want {
		t.Fatalf("codexHomeDirFor() = %q, want %q", got, want)
	}
}

func mustDecode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
}
