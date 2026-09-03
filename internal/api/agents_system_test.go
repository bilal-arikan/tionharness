package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentpkg "github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

func systemAgentAPIFixture(t *testing.T) (*Server, *workspace.Workspace, db.Agent) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureSystemAgents(context.Background(), agentpkg.SystemAgentDefaults()...); err != nil {
		t.Fatalf("ensure system agents: %v", err)
	}
	systemAgent, ok := database.FindAgentBySystemKey("titler")
	if !ok {
		t.Fatal("titler system agent not found")
	}
	if !systemAgent.Locked {
		t.Fatalf("seeded titler must be a locked built-in: %+v", *systemAgent)
	}
	s := newTestServer()
	s.runs = newChatRuns()
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1"}, DB: database}
	return s, wsp, *systemAgent
}

func systemAgentAPIRequest(wsp *workspace.Workspace, method, target, id string, body []byte) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	req.SetPathValue("id", id)
	return req
}

// TestHandleDeriveAgentBindsRole: deriving a built-in with bindRole yields the
// workspace's customisation of that role; edits pin overrides, and
// restore-default drops them so the child inherits the built-in again.
func TestHandleDeriveAgentBindsRole(t *testing.T) {
	s, wsp, builtin := systemAgentAPIFixture(t)

	rec := httptest.NewRecorder()
	s.handleDeriveAgent(rec, systemAgentAPIRequest(wsp, http.MethodPost, "/api/agents/"+builtin.ID+"/derive", builtin.ID, []byte(`{"bindRole":true}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var child db.Agent
	if err := json.Unmarshal(rec.Body.Bytes(), &child); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if child.ParentID != builtin.ID || !child.System || child.SystemKey != "titler" || child.Locked || len(child.Overrides) != 0 {
		t.Fatalf("derived child = %+v, want an unlocked child bound to titler", child)
	}
	if child.Name != "Titler (özel)" || child.Soul != builtin.Soul {
		t.Fatalf("derived child name=%q soul inherited=%v", child.Name, child.Soul == builtin.Soul)
	}
	if serving, _ := wsp.DB.FindAgentBySystemKey("titler"); serving.ID != child.ID {
		t.Fatalf("role should resolve to the customisation, got %q", serving.ID)
	}

	// A second bound customisation is refused while this one is enabled.
	rec = httptest.NewRecorder()
	s.handleDeriveAgent(rec, systemAgentAPIRequest(wsp, http.MethodPost, "/api/agents/"+builtin.ID+"/derive", builtin.ID, []byte(`{"bindRole":true}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("second bound derive: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}

	// Editing the child pins an override.
	rec = httptest.NewRecorder()
	s.handleUpdateAgent(rec, systemAgentAPIRequest(wsp, http.MethodPut, "/api/agents/"+child.ID, child.ID, []byte(`{"soul":"MUTATED"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("update child: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	updated, _ := wsp.DB.GetAgent(context.Background(), child.ID)
	if updated.Soul != "MUTATED" || len(updated.Overrides) != 1 || updated.Overrides[0] != "soul" {
		t.Fatalf("override not recorded: soul=%q overrides=%v", updated.Soul, updated.Overrides)
	}

	// Restore drops it.
	rec = httptest.NewRecorder()
	s.handleRestoreAgentDefaults(rec, systemAgentAPIRequest(wsp, http.MethodPost, "/api/agents/"+child.ID+"/restore-default", child.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("restore: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var restored db.Agent
	if err := json.Unmarshal(rec.Body.Bytes(), &restored); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if restored.Soul != builtin.Soul || len(restored.Overrides) != 0 {
		t.Fatalf("restore did not clear overrides: soul=%q overrides=%v", restored.Soul, restored.Overrides)
	}
}

func TestHandleRestoreDefaultsOnLockedAndRoot(t *testing.T) {
	s, wsp, builtin := systemAgentAPIFixture(t)
	rec := httptest.NewRecorder()
	s.handleRestoreAgentDefaults(rec, systemAgentAPIRequest(wsp, http.MethodPost, "/api/agents/"+builtin.ID+"/restore-default", builtin.ID, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("locked built-in: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	regular, err := wsp.DB.CreateAgent(context.Background(), db.Agent{Name: "Regular"})
	if err != nil {
		t.Fatalf("create regular agent: %v", err)
	}
	rec = httptest.NewRecorder()
	s.handleRestoreAgentDefaults(rec, systemAgentAPIRequest(wsp, http.MethodPost, "/api/agents/"+regular.ID+"/restore-default", regular.ID, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("root agent: expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteSystemAgentConflict(t *testing.T) {
	s, wsp, systemAgent := systemAgentAPIFixture(t)
	rec := httptest.NewRecorder()
	s.handleDeleteAgent(rec, systemAgentAPIRequest(wsp, http.MethodDelete, "/api/agents/"+systemAgent.ID, systemAgent.ID, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "built-in agent cannot be deleted") {
		t.Fatalf("expected actionable error, got: %s", rec.Body.String())
	}
}

// TestHandleUpdateLockedAgentConflict: every profile write to a built-in is a
// 409, and the tools endpoint follows the same rule.
func TestHandleUpdateLockedAgentConflict(t *testing.T) {
	s, wsp, systemAgent := systemAgentAPIFixture(t)
	rec := httptest.NewRecorder()
	s.handleUpdateAgent(rec, systemAgentAPIRequest(wsp, http.MethodPut, "/api/agents/"+systemAgent.ID, systemAgent.ID, []byte(`{"soul":"x"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	stored, _ := wsp.DB.GetAgent(context.Background(), systemAgent.ID)
	if stored.Soul != systemAgent.Soul {
		t.Fatal("locked agent was modified")
	}
}

func TestHandleUpdateAgentRejectsSystemIdentityChanges(t *testing.T) {
	s, wsp, systemAgent := systemAgentAPIFixture(t)
	tests := []struct {
		name    string
		body    string
		message string
	}{
		{name: "system", body: `{"system":false}`, message: "system cannot be changed"},
		{name: "systemKey", body: `{"systemKey":""}`, message: "systemKey cannot be changed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.handleUpdateAgent(rec, systemAgentAPIRequest(wsp, http.MethodPut, "/api/agents/"+systemAgent.ID, systemAgent.ID, []byte(tt.body)))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.message) {
				t.Fatalf("expected explicit immutable-field error %q, got: %s", tt.message, rec.Body.String())
			}

			stored, err := wsp.DB.GetAgent(context.Background(), systemAgent.ID)
			if err != nil {
				t.Fatalf("read agent after rejected update: %v", err)
			}
			if stored.System != systemAgent.System || stored.SystemKey != systemAgent.SystemKey {
				t.Fatalf("system identity persisted after rejected update: system=%v systemKey=%q", stored.System, systemAgent.SystemKey)
			}
		})
	}
}

// TestHandleUpdateAgentReparentAndReset covers the parentId/resetFields patch
// fields end to end, including the cycle guard's status code.
func TestHandleUpdateAgentReparentAndReset(t *testing.T) {
	s, wsp, _ := systemAgentAPIFixture(t)
	ctx := context.Background()
	base, _ := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Base", Soul: "base soul", Provider: "claude-cli", ThinkingLevel: "high"})
	kid, _ := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Kid", Soul: "kid soul", Provider: "claude-cli", ThinkingLevel: "high"})

	rec := httptest.NewRecorder()
	s.handleUpdateAgent(rec, systemAgentAPIRequest(wsp, http.MethodPut, "/api/agents/"+kid.ID, kid.ID, []byte(`{"parentId":"`+base.ID+`"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("reparent: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.handleUpdateAgent(rec, systemAgentAPIRequest(wsp, http.MethodPut, "/api/agents/"+kid.ID, kid.ID, []byte(`{"resetFields":["soul"]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("reset: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got, _ := wsp.DB.GetAgent(ctx, kid.ID)
	if got.Soul != "base soul" {
		t.Fatalf("soul after reset = %q, want inherited", got.Soul)
	}
	rec = httptest.NewRecorder()
	s.handleUpdateAgent(rec, systemAgentAPIRequest(wsp, http.MethodPut, "/api/agents/"+base.ID, base.ID, []byte(`{"parentId":"`+kid.ID+`"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cycle: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.handleUpdateAgent(rec, systemAgentAPIRequest(wsp, http.MethodPut, "/api/agents/"+kid.ID, kid.ID, []byte(`{"resetFields":["bogus"]}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown reset key: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
