package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentpkg "github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/prompts"
)

// A locked built-in reports the prompt compiled into the binary for its role.
func TestAgentBuiltinPromptOnLockedBuiltin(t *testing.T) {
	s, wsp, builtin := systemAgentAPIFixture(t)

	rec := httptest.NewRecorder()
	s.handleAgentBuiltinPrompt(rec, systemAgentAPIRequest(wsp, http.MethodGet, "/api/agents/"+builtin.ID+"/builtin-prompt", builtin.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got builtinPromptTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SystemKey != "titler" {
		t.Errorf("systemKey = %q, want %q", got.SystemKey, "titler")
	}
	if want := prompts.Default("title"); got.Soul != want {
		t.Errorf("soul = %q, want the compiled-in title prompt %q", got.Soul, want)
	}
}

// The point of the endpoint: a customisation whose soul has drifted still
// reports the ORIGINAL compiled-in text, so the editor can offer a revert.
func TestAgentBuiltinPromptOnDriftedCustomization(t *testing.T) {
	s, wsp, builtin := systemAgentAPIFixture(t)

	rec := httptest.NewRecorder()
	s.handleDeriveAgent(rec, systemAgentAPIRequest(wsp, http.MethodPost, "/api/agents/"+builtin.ID+"/derive", builtin.ID, []byte(`{"bindRole":true}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("derive: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var child db.Agent
	if err := json.Unmarshal(rec.Body.Bytes(), &child); err != nil {
		t.Fatalf("decode child: %v", err)
	}

	edited := "tamamen farkli bir prompt"
	if _, err := wsp.DB.UpdateAgent(t.Context(), child.ID, db.AgentProfilePatch{Soul: &edited}); err != nil {
		t.Fatalf("edit soul: %v", err)
	}

	rec = httptest.NewRecorder()
	s.handleAgentBuiltinPrompt(rec, systemAgentAPIRequest(wsp, http.MethodGet, "/api/agents/"+child.ID+"/builtin-prompt", child.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got builtinPromptTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Soul == edited {
		t.Fatal("endpoint returned the edited soul; it must return the compiled-in prompt")
	}
	if want := prompts.Default("title"); got.Soul != want {
		t.Errorf("soul = %q, want %q", got.Soul, want)
	}
}

// A plain user agent carries no system role, so there is no code prompt to
// revert to — 404 rather than a misleading empty string.
func TestAgentBuiltinPromptOnPlainAgentIs404(t *testing.T) {
	s, wsp, _ := systemAgentAPIFixture(t)

	plain, err := wsp.DB.CreateAgent(t.Context(), db.Agent{
		Name: "Serbest", Soul: "kendi promptu", Provider: "claude-cli",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rec := httptest.NewRecorder()
	s.handleAgentBuiltinPrompt(rec, systemAgentAPIRequest(wsp, http.MethodGet, "/api/agents/"+plain.ID+"/builtin-prompt", plain.ID, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Every prompt the registry marks as system-owned must resolve to a built-in
// definition, otherwise the revert button would 404 on a real system agent.
func TestEverySystemOwnedPromptHasBuiltinDefinition(t *testing.T) {
	for _, spec := range prompts.Specs() {
		if spec.OwnedBySystemKey == "" {
			continue
		}
		def, ok := agentpkg.SystemAgentDefault(spec.OwnedBySystemKey)
		if !ok {
			t.Errorf("prompt %q is owned by %q, which has no built-in definition", spec.Key, spec.OwnedBySystemKey)
			continue
		}
		if def.SystemPrompt != prompts.Default(spec.Key) {
			t.Errorf("built-in %q prompt does not match registry default for %q", spec.OwnedBySystemKey, spec.Key)
		}
	}
}
