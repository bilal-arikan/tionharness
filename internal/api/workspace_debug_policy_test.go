package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/settings"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// disableDebugJournal turns the journal off with a non-default cap and pushes
// the change into the live subsystems, mirroring a settings update at runtime.
func disableDebugJournal(t *testing.T, s *Server) int {
	t.Helper()
	enabled, capValue := false, 137
	if _, err := s.settings.Apply(settings.Patch{
		DebugJournalEnabled: &enabled,
		DebugJournalCap:     &capValue,
	}); err != nil {
		t.Fatalf("apply settings: %v", err)
	}
	s.applySettings()
	return capValue
}

// assertJournalPolicy fails when a workspace store did not inherit the user's
// debug-journal setting.
func assertJournalPolicy(t *testing.T, wsp *workspace.Workspace, wantCap int) {
	t.Helper()
	if wsp == nil || wsp.DB == nil {
		t.Fatalf("workspace store is not open")
	}
	if wsp.DB.DebugJournalEnabled() {
		t.Fatalf("journal must stay off for a runtime workspace")
	}
	if got := wsp.DB.DebugJournalCap(); got != wantCap {
		t.Fatalf("journal cap = %d, want %d", got, wantCap)
	}
}

// TestCreatedWorkspaceInheritsDebugJournalPolicy guards the fail-open bug: a
// workspace created after boot opens its store past the last applySettings pass,
// so without an explicit push it kept the zero-value policy (journal ON, default
// cap) and silently ignored a user who had turned the journal off.
func TestCreatedWorkspaceInheritsDebugJournalPolicy(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	wantCap := disableDebugJournal(t, s)

	req := httptest.NewRequest(http.MethodPost, "/api/workspaces", strings.NewReader(`{"name":"runtime"}`))
	rec := httptest.NewRecorder()
	s.handleCreateWorkspace(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create workspace: %d %s", rec.Code, rec.Body.String())
	}
	var created createWorkspaceResp
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	wsp, err := s.workspaces.Get(created.ID)
	if err != nil {
		t.Fatalf("get created workspace: %v", err)
	}
	assertJournalPolicy(t, wsp, wantCap)
}

// TestAttachedWorkspaceInheritsDebugJournalPolicy is the same guard for the
// adopt-an-existing-folder path.
func TestAttachedWorkspaceInheritsDebugJournalPolicy(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	wantCap := disableDebugJournal(t, s)

	// A minimal on-disk workspace: Attach only requires the store/ subfolder.
	adopted := filepath.Join(t.TempDir(), "adopted")
	if err := os.MkdirAll(filepath.Join(adopted, "store"), 0o755); err != nil {
		t.Fatalf("prepare workspace dir: %v", err)
	}
	body, err := json.Marshal(attachWorkspaceReq{Path: adopted})
	if err != nil {
		t.Fatalf("encode attach request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces/attach", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.handleAttachWorkspace(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("attach workspace: %d %s", rec.Code, rec.Body.String())
	}
	var meta workspace.Meta
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode attach response: %v", err)
	}
	wsp, err := s.workspaces.Get(meta.ID)
	if err != nil {
		t.Fatalf("get attached workspace: %v", err)
	}
	assertJournalPolicy(t, wsp, wantCap)
}
