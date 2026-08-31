package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestCreateAgentKeepsEmptyModelAsDeliberateChoice pins TSK567: an empty model
// on POST /api/agents is a REAL choice (the catalog's ID:"" entry — "claude
// oturum modeli", which makes the CLI provider omit --model), not "unspecified".
// It must therefore be stored as-is instead of being silently overwritten with
// another agent's model.
func TestCreateAgentKeepsEmptyModelAsDeliberateChoice(t *testing.T) {
	s, wsp := agentStrictFixture(t)

	// An existing agent whose model would previously have been copied over.
	if _, err := wsp.DB.CreateAgent(context.Background(), db.Agent{
		Name: "Existing", Provider: "claude-cli", Model: "claude-opus-5",
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	body := []byte(`{"name":"SessionModel","provider":"claude-cli","model":"","thinkingLevel":"medium"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()

	s.handleCreateAgent(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created db.Agent
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
	}
	if created.Model != "" {
		t.Fatalf("empty model must be stored as-is (session model), got %q", created.Model)
	}
	// The provider fallback is a separate mechanism and stays intact.
	if created.Provider != "claude-cli" {
		t.Fatalf("provider = %q, want claude-cli", created.Provider)
	}
}

// TestCreateAgentKeepsExplicitModel is the companion: an explicitly requested
// model is stored untouched, whatever other agents use.
func TestCreateAgentKeepsExplicitModel(t *testing.T) {
	s, wsp := agentStrictFixture(t)

	if _, err := wsp.DB.CreateAgent(context.Background(), db.Agent{
		Name: "Existing", Provider: "claude-cli", Model: "claude-opus-5",
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	body := []byte(`{"name":"Pinned","provider":"claude-cli","model":"claude-sonnet-5","thinkingLevel":"medium"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()

	s.handleCreateAgent(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created db.Agent
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
	}
	if created.Model != "claude-sonnet-5" {
		t.Fatalf("model = %q, want claude-sonnet-5", created.Model)
	}
}
