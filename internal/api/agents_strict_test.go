package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// agentStrictFixture reuses the full newWorkspaceServer harness (real settings
// store + Runtime) — handleCreateAgent reads s.settings.Get() unconditionally,
// and handleUpdateAgent dereferences wsp.Runtime when coordinatorWorkflow is set.
func agentStrictFixture(t *testing.T) (*Server, *workspace.Workspace) {
	t.Helper()
	return newWorkspaceServer(t)
}

// TestHandleCreateAgentRejectsWorkingDir pins the regression this change fixes:
// workingDir is a Session field (db.Session.WorkingDir), not an Agent field. A
// caller that sends it on POST /api/agents must get a 400 naming the field and
// pointing at where it belongs, not a silent 200 that drops it.
func TestHandleCreateAgentRejectsWorkingDir(t *testing.T) {
	s, wsp := agentStrictFixture(t)

	body := []byte(`{"name":"Ada","provider":"claude-cli","workingDir":"C:\\repo"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()

	s.handleCreateAgent(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown field") || !strings.Contains(rec.Body.String(), "workingDir") {
		t.Fatalf("expected field name in error, got: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "session property") {
		t.Fatalf("expected guidance pointing at the session, got: %s", rec.Body.String())
	}
}

// TestHandleCreateAgentValidBodyStillWorks is the no-regression companion: a
// body using only real createAgentReq fields must still succeed with strict
// decoding on.
func TestHandleCreateAgentValidBodyStillWorks(t *testing.T) {
	s, wsp := agentStrictFixture(t)

	body := []byte(`{"name":"Ada","soul":"helpful","provider":"claude-cli","mcpEnabled":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()

	s.handleCreateAgent(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandleUpdateAgentRejectsWorkingDir mirrors the create-side regression test
// for PUT /api/agents/{id}.
func TestHandleUpdateAgentRejectsWorkingDir(t *testing.T) {
	s, wsp := agentStrictFixture(t)
	agent, err := wsp.DB.CreateAgent(context.Background(), db.Agent{Name: "Ada", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	body := []byte(`{"workingDir":"C:\\repo"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	req.SetPathValue("id", agent.ID)
	rec := httptest.NewRecorder()

	s.handleUpdateAgent(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown field") || !strings.Contains(rec.Body.String(), "workingDir") {
		t.Fatalf("expected field name in error, got: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "session property") {
		t.Fatalf("expected guidance pointing at the session, got: %s", rec.Body.String())
	}
}

// TestHandleUpdateAgentValidBodyStillWorks is the no-regression companion for
// PUT /api/agents/{id}.
func TestHandleUpdateAgentValidBodyStillWorks(t *testing.T) {
	s, wsp := agentStrictFixture(t)
	agent, err := wsp.DB.CreateAgent(context.Background(), db.Agent{Name: "Ada", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	body := []byte(`{"name":"Renamed","color":"#fff"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	req.SetPathValue("id", agent.ID)
	rec := httptest.NewRecorder()

	s.handleUpdateAgent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
