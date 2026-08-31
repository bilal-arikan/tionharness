package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// postAgent runs handleCreateAgent against the shared workspace fixture.
func postAgent(t *testing.T, s *Server, wsp *workspace.Workspace, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleCreateAgent(rec, req)
	return rec
}

// putAgent runs handleUpdateAgent for id against the shared workspace fixture.
func putAgent(t *testing.T, s *Server, wsp *workspace.Workspace, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/agents/"+id, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleUpdateAgent(rec, req)
	return rec
}

// TestCreateAgentRequiresThinkingLevel: a blank level is the ambiguous legacy
// state, so it must come back as a 400 rather than being defaulted silently.
func TestCreateAgentRequiresThinkingLevel(t *testing.T) {
	s, wsp := newWorkspaceServer(t)

	rec := postAgent(t, s, wsp, `{"name":"Ada","provider":"claude-cli"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "thinkingLevel is required") {
		t.Fatalf("error should name the field: %s", rec.Body.String())
	}
}

// TestCreateAgentRejectsUnsupportedThinkingLevel covers the model/level
// agreement check: a tier the model does not reason at is a silent no-op, so it
// is refused with the tiers that would have worked.
func TestCreateAgentRejectsUnsupportedThinkingLevel(t *testing.T) {
	s, wsp := newWorkspaceServer(t)

	rec := postAgent(t, s, wsp,
		`{"name":"Ada","provider":"claude-cli","model":"deepseek-v4-flash","thinkingLevel":"high"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "supported: off") {
		t.Fatalf("error should list the supported tiers: %s", rec.Body.String())
	}

	// The same model with the one tier it does support goes through.
	ok := postAgent(t, s, wsp,
		`{"name":"Ada","provider":"claude-cli","model":"deepseek-v4-flash","thinkingLevel":"off"}`)
	if ok.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", ok.Code, ok.Body.String())
	}
}

// TestUpdateAgentRejectsBlankAndUnsupportedThinkingLevel: an explicit "" in the
// patch would restore the ambiguous state, and a tier is judged against the
// model the SAME patch installs.
func TestUpdateAgentRejectsBlankAndUnsupportedThinkingLevel(t *testing.T) {
	s, wsp := newWorkspaceServer(t)

	created := postAgent(t, s, wsp,
		`{"name":"Ada","provider":"claude-cli","model":"claude-opus-4-8","thinkingLevel":"high"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", created.Code, created.Body.String())
	}
	var agent db.Agent
	if err := json.Unmarshal(created.Body.Bytes(), &agent); err != nil {
		t.Fatalf("decode created agent: %v", err)
	}

	blank := putAgent(t, s, wsp, agent.ID, `{"thinkingLevel":""}`)
	if blank.Code != http.StatusBadRequest {
		t.Fatalf("blank level: expected 400, got %d: %s", blank.Code, blank.Body.String())
	}

	// max is fine on the stored adaptive model...
	okMax := putAgent(t, s, wsp, agent.ID, `{"thinkingLevel":"max"}`)
	if okMax.Code != http.StatusOK {
		t.Fatalf("max on adaptive model: expected 200, got %d: %s", okMax.Code, okMax.Body.String())
	}
	// ...but not when the same patch moves the agent onto a legacy model.
	clash := putAgent(t, s, wsp, agent.ID, `{"model":"claude-opus-4-6","thinkingLevel":"max"}`)
	if clash.Code != http.StatusBadRequest {
		t.Fatalf("max on legacy model: expected 400, got %d: %s", clash.Code, clash.Body.String())
	}

	// A patch that leaves the level alone is unaffected.
	rename := putAgent(t, s, wsp, agent.ID, `{"name":"Ada II"}`)
	if rename.Code != http.StatusOK {
		t.Fatalf("unrelated patch: expected 200, got %d: %s", rename.Code, rename.Body.String())
	}
}
