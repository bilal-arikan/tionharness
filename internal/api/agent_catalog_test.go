package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestAgentCatalogRoleAssignmentsRemainUnique(t *testing.T) {
	s, first := newWorkspaceServer(t)
	catalog := s.workspaces.AgentCatalog()
	base, ok := catalog.FindBuiltinAgentBySystemKey("titler")
	if !ok {
		t.Fatal("missing built-in")
	}
	a, err := catalog.DeriveAgent(t.Context(), base.ID, db.DeriveAgentOptions{Name: "First role", BindRole: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := catalog.DeriveAgent(t.Context(), base.ID, db.DeriveAgentOptions{Name: "Second role", BindRole: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.DB.AssignCatalogAgent(t.Context(), a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := first.DB.AssignCatalogAgent(t.Context(), b.ID); !errors.Is(err, db.ErrSystemRoleTaken) {
		t.Fatalf("conflicting assignment accepted: %v", err)
	}
	disabled := true
	if _, err := catalog.UpdateAgent(t.Context(), a.ID, db.AgentProfilePatch{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := first.DB.AssignCatalogAgent(t.Context(), b.ID); err != nil {
		t.Fatal(err)
	}
	disabled = false
	if _, err := catalog.UpdateAgent(t.Context(), a.ID, db.AgentProfilePatch{Disabled: &disabled}); !errors.Is(err, db.ErrSystemRoleTaken) {
		t.Fatalf("conflicting enable accepted: %v", err)
	}
}

func TestAgentCatalogDeleteKeepsHistoryAndRemovesAssignments(t *testing.T) {
	s, first := newWorkspaceServer(t)
	a, err := first.DB.CreateAgent(t.Context(), db.Agent{Name: "Keep history"})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.catalogHandler(s.handleDeleteCatalogAgent, false)(rec, systemAgentAPIRequest(first, http.MethodDelete, "/api/agent-catalog/"+a.CatalogID, a.CatalogID, nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	local, err := first.DB.GetAgent(t.Context(), a.ID)
	if err != nil || local.Name != a.Name || !local.Deleted || local.RunnableErr() == nil {
		t.Fatalf("history not preserved: %+v %v", local, err)
	}
	if _, err := first.DB.AssignCatalogAgent(t.Context(), a.CatalogID); !errors.Is(err, db.ErrNotFound) {
		t.Fatal("deleted profile can be assigned")
	}
}

func TestAgentCatalogSharedEditRequiresConfirmation(t *testing.T) {
	s, first := newWorkspaceServer(t)
	second, err := s.workspaces.Create("Second workspace", "", "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := first.DB.CreateAgent(t.Context(), db.Agent{Name: "Shared", Provider: "claude-cli", Model: "sonnet", ThinkingLevel: "high"})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := second.DB.AssignCatalogAgent(t.Context(), a.CatalogID)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/agents/" + a.ID, "/api/agent-catalog/" + a.CatalogID} {
		id := a.ID
		handler := http.HandlerFunc(s.handleUpdateAgent)
		if path == "/api/agent-catalog/"+a.CatalogID {
			id = a.CatalogID
			handler = s.catalogHandler(handler, false)
		}
		request := systemAgentAPIRequest(first, http.MethodPut, path, id, []byte(`{"name":"Changed"}`))
		rec := httptest.NewRecorder()
		handler(rec, request)
		if rec.Code != http.StatusConflict {
			t.Fatalf("unconfirmed write: %d %s", rec.Code, rec.Body.String())
		}
		var response struct {
			ConfirmationRequired bool  `json:"confirmationRequired"`
			Workspaces           []any `json:"workspaces"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if !response.ConfirmationRequired || len(response.Workspaces) != 2 {
			t.Fatalf("missing impact: %s", rec.Body.String())
		}
		before, _ := first.DB.GetAgent(t.Context(), a.ID)
		if before.Name != "Shared" {
			t.Fatal("unconfirmed mutation persisted")
		}
	}
	req := systemAgentAPIRequest(second, http.MethodPut, "/api/agents/"+linked.ID, linked.ID, []byte(`{"name":"Confirmed"}`))
	req.Header.Set("X-Confirm-Shared-Agent", "true")
	rec := httptest.NewRecorder()
	s.handleUpdateAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirmed write: %d %s", rec.Code, rec.Body.String())
	}
	got, _ := first.DB.GetAgent(t.Context(), a.ID)
	if got.Name != "Confirmed" {
		t.Fatal("confirmed edit not shared")
	}
	for _, handler := range []http.HandlerFunc{s.handleSetAgentTools, s.handleRestoreAgentDefaults} {
		rec = httptest.NewRecorder()
		handler(rec, systemAgentAPIRequest(first, http.MethodPost, "/api/agents/"+a.ID, a.ID, []byte(`{}`)))
		if rec.Code != http.StatusConflict {
			t.Fatalf("auxiliary write bypassed confirmation: %d", rec.Code)
		}
	}
}

func TestAgentCatalogListsEveryWorkspaceAndUnassigned(t *testing.T) {
	s, first := newWorkspaceServer(t)
	second, err := s.workspaces.Create("Other", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []*db.DB{first.DB, second.DB, s.workspaces.AgentCatalog()} {
		if _, err := store.CreateAgent(t.Context(), db.Agent{Name: "Independent"}); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	s.handleAgentCatalog(rec, httptest.NewRequest(http.MethodGet, "/api/agent-catalog", nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var result struct {
		Agents []struct {
			Agent       db.Agent `json:"agent"`
			Assignments []any    `json:"assignments"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	custom, unassigned := 0, 0
	for _, row := range result.Agents {
		if row.Agent.Name == "Independent" {
			custom++
			if len(row.Assignments) == 0 {
				unassigned++
			}
		}
	}
	if custom != 3 || unassigned != 1 {
		t.Fatalf("catalog aggregation: custom=%d unassigned=%d", custom, unassigned)
	}
}
