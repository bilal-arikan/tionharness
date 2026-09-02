package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// TestNewestFlowRunStatusPicksTheFirstPerFlow: the index relies on its input
// being newest-first (what ListFlowRuns returns), so the first row seen for a
// flow wins and later ones are ignored.
func TestNewestFlowRunStatusPicksTheFirstPerFlow(t *testing.T) {
	got := newestFlowRunStatus([]db.FlowRun{
		{ID: "RUN3", FlowID: "FLW1", Status: db.FlowRunning},
		{ID: "RUN2", FlowID: "FLW1", Status: db.FlowFailure},
		{ID: "RUN1", FlowID: "FLW1", Status: db.FlowSuccess},
		{ID: "RUN9", FlowID: "FLW2", Status: db.FlowSuccess},
	})
	if got["FLW1"] != db.FlowRunning {
		t.Errorf("FLW1 = %q, want the newest run's status %q", got["FLW1"], db.FlowRunning)
	}
	if got["FLW2"] != db.FlowSuccess {
		t.Errorf("FLW2 = %q, want %q", got["FLW2"], db.FlowSuccess)
	}
	if _, ok := got["FLW-none"]; ok {
		t.Error("index invented an entry for a flow with no runs")
	}
}

// TestExecutionsReportsNewestFlowRunStatus is the end-to-end guard that the
// one-scan index produces the same answer the old per-session ListFlowRuns did:
// a flow session's lastStatus is its flow's NEWEST run status, and a flow with
// no runs reports nothing.
func TestExecutionsReportsNewestFlowRunStatus(t *testing.T) {
	ctx := context.Background()
	s, wsp := newWorkspaceServer(t)

	flow, err := wsp.DB.CreateFlow(ctx, db.Flow{Name: "f"})
	if err != nil {
		t.Fatalf("CreateFlow: %v", err)
	}
	old, err := wsp.DB.CreateFlowRun(ctx, db.FlowRun{FlowID: flow.ID})
	if err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}
	if err := wsp.DB.FinishFlowRun(ctx, old.ID, db.FlowFailure, "", "boom"); err != nil {
		t.Fatalf("FinishFlowRun: %v", err)
	}
	newer, err := wsp.DB.CreateFlowRun(ctx, db.FlowRun{FlowID: flow.ID})
	if err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}
	if err := wsp.DB.FinishFlowRun(ctx, newer.ID, db.FlowSuccess, "", ""); err != nil {
		t.Fatalf("FinishFlowRun: %v", err)
	}

	// A flow-kind session pointing at that flow, plus an empty flow to prove the
	// index does not invent statuses.
	if _, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "flow", SourceID: flow.ID, Title: "run"}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "flow", SourceID: "FLW-none", Title: "empty"}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	byTitle := map[string]string{}
	for _, item := range listExecutions(t, s, wsp) {
		byTitle[item.Title] = item.LastStatus
	}
	if byTitle["run"] != db.FlowSuccess {
		t.Errorf("lastStatus = %q, want the newest run's %q", byTitle["run"], db.FlowSuccess)
	}
	if byTitle["empty"] != "" {
		t.Errorf("flow with no runs reported lastStatus %q, want empty", byTitle["empty"])
	}
}

// TestExecutionsSkipsFlowIndexWithoutFlowSessions: a workspace whose feed has no
// flow session must not pay for the store scan at all — and must still answer.
func TestExecutionsSkipsFlowIndexWithoutFlowSessions(t *testing.T) {
	ctx := context.Background()
	s, wsp := newWorkspaceServer(t)

	if _, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "c"}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	items := listExecutions(t, s, wsp)
	if len(items) == 0 {
		t.Fatal("chat-only feed came back empty")
	}
	for _, item := range items {
		if item.LastStatus != "" {
			t.Errorf("chat session %q got lastStatus %q, want empty", item.Title, item.LastStatus)
		}
	}
}

func TestExecutionsIncludesCoordinatorLineage(t *testing.T) {
	ctx := context.Background()
	s, wsp := newWorkspaceServer(t)

	root, err := wsp.DB.CreateSession(ctx, db.Session{Title: "root", CoordinatorMode: true})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	child, err := wsp.DB.CreateSession(ctx, db.Session{
		Title:                    "child coordinator",
		Role:                     "worker",
		CoordinatorMode:          true,
		CoordinatorSessionID:     root.ID,
		RootCoordinatorSessionID: root.ID,
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	leaf, err := wsp.DB.CreateSession(ctx, db.Session{
		Title:                    "leaf worker",
		Role:                     "worker",
		CoordinatorSessionID:     child.ID,
		RootCoordinatorSessionID: root.ID,
	})
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}

	var got *executionItem
	for _, item := range listExecutions(t, s, wsp) {
		if item.SessionID == leaf.ID {
			item := item
			got = &item
			break
		}
	}
	if got == nil {
		t.Fatalf("leaf execution %s not returned", leaf.ID)
	}
	if got.CoordinatorSessionID != child.ID {
		t.Errorf("coordinatorSessionId = %q, want %q", got.CoordinatorSessionID, child.ID)
	}
	if got.RootCoordinatorSessionID != root.ID {
		t.Errorf("rootCoordinatorSessionId = %q, want %q", got.RootCoordinatorSessionID, root.ID)
	}
}

// listExecutions drives the real handler in its legacy (unpaged) shape.
func listExecutions(t *testing.T, s *Server, wsp *workspace.Workspace) []executionItem {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/executions", nil)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleListExecutions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var out []executionItem
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	return out
}
