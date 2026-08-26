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

func TestHandleRestoreSystemAgentDefault(t *testing.T) {
	s, wsp, systemAgent := systemAgentAPIFixture(t)
	customPrompt, customModel := "MUTATED", "custom-model"
	if _, err := wsp.DB.UpdateAgent(context.Background(), systemAgent.ID, db.AgentProfilePatch{Soul: &customPrompt, Model: &customModel}); err != nil {
		t.Fatalf("customize system agent: %v", err)
	}
	if err := wsp.DB.UpdateAgentAllowedTools(context.Background(), systemAgent.ID, `["Bash"]`); err != nil {
		t.Fatalf("customize allowed tools: %v", err)
	}

	rec := httptest.NewRecorder()
	s.handleRestoreSystemAgent(rec, systemAgentAPIRequest(wsp, http.MethodPost, "/api/agents/"+systemAgent.ID+"/restore-default", systemAgent.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got db.Agent
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	def, _ := agentpkg.SystemAgentDefault(systemAgent.SystemKey)
	if def.SystemPrompt == "" {
		t.Fatal("compiled default prompt is empty")
	}
	if got.ID != systemAgent.ID || !got.System || got.SystemKey != systemAgent.SystemKey {
		t.Fatalf("system identity changed: %+v", got)
	}
	if got.Soul != def.SystemPrompt || got.Model != def.SuggestedModel || got.AllowedTools != def.AllowedTools {
		t.Fatalf("defaults not restored: %+v", got)
	}
	stored, err := wsp.DB.GetAgent(context.Background(), systemAgent.ID)
	if err != nil {
		t.Fatalf("read restored system agent: %v", err)
	}
	if stored.Soul != def.SystemPrompt || stored.Model != def.SuggestedModel {
		t.Fatalf("restored defaults not persisted: soul=%q model=%q", stored.Soul, stored.Model)
	}
}

func TestHandleRestoreSystemAgentDefaultWithoutSystemKey(t *testing.T) {
	s, wsp, _ := systemAgentAPIFixture(t)
	regular, err := wsp.DB.CreateAgent(context.Background(), db.Agent{Name: "Regular"})
	if err != nil {
		t.Fatalf("create regular agent: %v", err)
	}
	rec := httptest.NewRecorder()
	s.handleRestoreSystemAgent(rec, systemAgentAPIRequest(wsp, http.MethodPost, "/api/agents/"+regular.ID+"/restore-default", regular.ID, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteSystemAgentConflict(t *testing.T) {
	s, wsp, systemAgent := systemAgentAPIFixture(t)
	rec := httptest.NewRecorder()
	s.handleDeleteAgent(rec, systemAgentAPIRequest(wsp, http.MethodDelete, "/api/agents/"+systemAgent.ID, systemAgent.ID, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "system agent cannot be deleted; disable it instead") {
		t.Fatalf("expected actionable error, got: %s", rec.Body.String())
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
				t.Fatalf("system identity persisted after rejected update: system=%v systemKey=%q", stored.System, stored.SystemKey)
			}
		})
	}
}
