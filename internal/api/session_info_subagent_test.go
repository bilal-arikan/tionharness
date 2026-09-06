package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// sessionInfo issues GET /api/sessions/<id>/info and returns the recorder.
func sessionInfo(t *testing.T, s *Server, wsID, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID+"/info", nil)
	req.Header.Set("X-Workspace-Id", wsID)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

// subagentChildOf creates a chat parent plus one delegated child of it, shaped
// the way a pre-fix run_subagent row was: identified by its target only.
func subagentChildOf(t *testing.T, database *db.DB, child db.Session) (string, string) {
	t.Helper()
	ctx := context.Background()
	owner, err := database.CreateAgent(ctx, db.Agent{Name: "Kâşif"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	parent, err := database.CreateSession(ctx, db.Session{AgentID: owner.ID, Kind: "chat"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child.ParentSessionID = parent.ID
	child.Kind = "subagent"
	child.ExecutionType, child.Category = db.ExecutionSubagent, db.CategorySubagent
	child.ContextMode, child.Visibility = db.ContextIsolated, db.VisibilityInternal
	if child.TargetAgentID == "AGENT" {
		child.TargetAgentID = owner.ID
	}
	created, err := database.CreateChildSession(ctx, child)
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	return owner.ID, created.ID
}

// A delegated run identifies its agent as a TARGET, not an owner. Reading the
// owner alone made the whole info panel 404 with "agent not found", so the
// session could not be inspected at all.
func TestSessionInfoResolvesADelegatedRunsTargetAgent(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ownerID, childID := subagentChildOf(t, wsp.DB, db.Session{TargetAgentID: "AGENT"})

	rec := sessionInfo(t, s, wsp.ID, childID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var got struct {
		AgentID   string `json:"agentId"`
		AgentName string `json:"agentName"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AgentID != ownerID {
		t.Errorf("agentId = %q, want %q", got.AgentID, ownerID)
	}
	if got.AgentName != "Kâşif" {
		t.Errorf("agentName = %q, want %q", got.AgentName, "Kâşif")
	}
}

// A profile subagent runs through an ephemeral clone that is never persisted, so
// there is no agent row to resolve at all: it must still be inspectable, and
// named from the profile rather than left blank.
func TestSessionInfoNamesAProfileSubagentFromItsProfile(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	_, childID := subagentChildOf(t, wsp.DB, db.Session{TargetProfile: "coder"})

	rec := sessionInfo(t, s, wsp.ID, childID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var got struct {
		AgentName string `json:"agentName"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AgentName != "subagent:coder" {
		t.Errorf("agentName = %q, want %q", got.AgentName, "subagent:coder")
	}
}

// The executions feed reads the same identity, so a delegated run must not show
// up there as an unowned row either.
func TestExecutionsFeedNamesDelegatedRuns(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	subagentChildOf(t, wsp.DB, db.Session{TargetAgentID: "AGENT"})
	subagentChildOf(t, wsp.DB, db.Session{TargetProfile: "coder"})

	req := httptest.NewRequest(http.MethodGet, "/api/executions?kind=subagent", nil)
	req.Header.Set("X-Workspace-Id", wsp.ID)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var rows []struct {
		AgentName string `json:"agentName"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if row.AgentName == "" {
			t.Errorf("a delegated run came back with no agent name: %+v", rows)
		}
	}
}
