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

func goalAPIFixture(t *testing.T) (*Server, *workspace.Workspace, db.Goal) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	min := 0.9
	g, err := database.CreateGoal(context.Background(), db.Goal{
		Name: "Ucuz inceleme", RawText: "incelemeler pahalı",
		Primary:    db.GoalMetric{Metric: "recipe.avgCostUSD", Direction: "min"},
		Guardrails: []db.GoalGuardrail{{Metric: "recipe.successRate", Min: &min}},
	}, db.GoalByWriter, "written")
	if err != nil {
		t.Fatalf("seed goal: %v", err)
	}
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1", Name: "Atölye"}, DB: database}
	return newTestServer(), wsp, g
}

func goalRequest(wsp *workspace.Workspace, method, target, id string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, target, &buf)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	if id != "" {
		req.SetPathValue("id", id)
	}
	return req
}

// TestGoalHandlers: list, direct create (validated), get, user edit
// (validated), status change, delete.
func TestGoalHandlers(t *testing.T) {
	s, wsp, g := goalAPIFixture(t)

	rec := httptest.NewRecorder()
	s.handleListGoals(rec, goalRequest(wsp, http.MethodGet, "/api/goals", "", nil))
	var list []db.Goal
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].ID != g.ID {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}

	// Direct create from the editor: validated, provenance is the user's, the
	// raw text is never set on this path; an unknown metric is a 422.
	rec = httptest.NewRecorder()
	s.handleCreateGoal(rec, goalRequest(wsp, http.MethodPost, "/api/goals", "", db.Goal{
		Name: "Hızlı kartlar", RawText: "tampered", CreatedBy: "someone",
		Primary: db.GoalMetric{Metric: "board.cycleTimeSec"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created db.Goal
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == "" || created.RawText != "" || created.CreatedBy != db.GoalByUser || created.Status != db.GoalStatusDraft || created.Primary.Direction != "min" {
		t.Fatalf("created = %+v", created)
	}
	rec = httptest.NewRecorder()
	s.handleCreateGoal(rec, goalRequest(wsp, http.MethodPost, "/api/goals", "", db.Goal{Name: "x", Primary: db.GoalMetric{Metric: "recipe.vibes"}}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create with unknown metric must be 422, got %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.handleDeleteGoal(rec, goalRequest(wsp, http.MethodDelete, "/api/goals/"+created.ID, created.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete created: %d", rec.Code)
	}

	// User edit: rename; invalid metric is a 422.
	edit := g
	edit.Name = "Kod incelemesi ucuzlasın"
	rec = httptest.NewRecorder()
	s.handleUpdateGoal(rec, goalRequest(wsp, http.MethodPut, "/api/goals/"+g.ID, g.ID, edit))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	var updated db.Goal
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Name != edit.Name || len(updated.History) != 2 || updated.History[1].By != db.GoalByUser {
		t.Fatalf("updated = %+v", updated)
	}
	bad := updated
	bad.Primary.Metric = "recipe.vibes"
	rec = httptest.NewRecorder()
	s.handleUpdateGoal(rec, goalRequest(wsp, http.MethodPut, "/api/goals/"+g.ID, g.ID, bad))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid metric must be 422, got %d %s", rec.Code, rec.Body.String())
	}

	// Now activation succeeds; an unknown status is a 400.
	rec = httptest.NewRecorder()
	s.handleSetGoalStatus(rec, goalRequest(wsp, http.MethodPost, "/api/goals/"+g.ID+"/status", g.ID, map[string]string{"status": "active"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("activate: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.handleSetGoalStatus(rec, goalRequest(wsp, http.MethodPost, "/api/goals/"+g.ID+"/status", g.ID, map[string]string{"status": "done"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown status: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleListGoals(rec, goalRequest(wsp, http.MethodGet, "/api/goals?status=active", "", nil))
	list = nil
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Status != db.GoalStatusActive {
		t.Fatalf("status filter: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	s.handleGetGoal(rec, goalRequest(wsp, http.MethodGet, "/api/goals/GOL99", "GOL99", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing goal: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleDeleteGoal(rec, goalRequest(wsp, http.MethodDelete, "/api/goals/"+g.ID, g.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleListGoals(rec, goalRequest(wsp, http.MethodGet, "/api/goals", "", nil))
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("list after delete = %s", rec.Body.String())
	}
}

// TestGoalIntakeRejectsEmpty: an empty statement never reaches the model.
func TestGoalIntakeRejectsEmpty(t *testing.T) {
	s, wsp, _ := goalAPIFixture(t)
	rec := httptest.NewRecorder()
	s.handleGoalIntake(rec, goalRequest(wsp, http.MethodPost, "/api/goals/intake", "", "not json"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad body: %d", rec.Code)
	}
}
