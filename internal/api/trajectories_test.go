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

func TestTrajectoryReadEndpoints(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	ag, err := database.CreateAgent(ctx, db.Agent{Name: "Builder"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	root, err := database.CreateSession(ctx, db.Session{AgentID: ag.ID, Title: "Root"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	created, err := database.CreateTrajectory(ctx, db.Trajectory{
		RootSessionID: root.ID, TemplateRef: "plan-dev-test@1", Status: db.TrajStatusRunning,
		Nodes: []db.TrajectoryNode{{ID: "p:plan", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: db.TrajStateActive}},
	})
	if err != nil {
		t.Fatalf("create trajectory: %v", err)
	}

	server := newTestServer()
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1", Name: "Test"}, DB: database}
	call := func(path string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
		rec := httptest.NewRecorder()
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/trajectories", server.handleListTrajectories)
		mux.HandleFunc("GET /api/trajectories/{id}", server.handleGetTrajectory)
		mux.ServeHTTP(rec, req)
		_ = handler
		return rec
	}

	rec := call("/api/trajectories", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", rec.Code, rec.Body.String())
	}
	var rows []db.TrajectoryIndexEntry
	if err := json.NewDecoder(rec.Body).Decode(&rows); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != created.ID || rows[0].NodeCount != 1 {
		t.Fatalf("list rows = %+v", rows)
	}

	rec = call("/api/trajectories?root=nope", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("filtered list = %d %q (want empty array)", rec.Code, rec.Body.String())
	}

	rec = call("/api/trajectories/"+created.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d: %s", rec.Code, rec.Body.String())
	}
	var got db.Trajectory
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.ID != created.ID || len(got.Nodes) != 1 || got.Nodes[0].ID != "p:plan" {
		t.Fatalf("get = %+v", got)
	}

	rec = call("/api/trajectories/RTA999", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", rec.Code)
	}
}
