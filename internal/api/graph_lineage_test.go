package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// Lineage edges (_Docs/77 R9): coordinator → worker is "spawned", a session →
// the new root it started is "forked_from"; both endpoints must be in scope,
// and ?scope=recent widens the payload to recently-active idle sessions.
func TestWorkspaceGraphLineageEdges(t *testing.T) {
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
	coord, err := database.CreateSession(ctx, db.Session{AgentID: ag.ID, Title: "Coord"})
	if err != nil {
		t.Fatalf("create coord: %v", err)
	}
	worker, err := database.CreateSession(ctx, db.Session{AgentID: ag.ID, Kind: "worker", Title: "Worker", CoordinatorSessionID: coord.ID})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}
	// A handoff continuation started from the worker: forked_from.
	fork, err := database.CreateSession(ctx, db.Session{AgentID: ag.ID, Title: "Fork", ParentSessionID: worker.ID})
	if err != nil {
		t.Fatalf("create fork: %v", err)
	}
	// Its trigger is idle, so this fork's edge is dropped in live scope and
	// only appears under ?scope=recent.
	idle, err := database.CreateSession(ctx, db.Session{AgentID: ag.ID, Title: "Idle"})
	if err != nil {
		t.Fatalf("create idle: %v", err)
	}
	fromIdle, err := database.CreateSession(ctx, db.Session{AgentID: ag.ID, Title: "FromIdle", ParentSessionID: idle.ID})
	if err != nil {
		t.Fatalf("create fromIdle: %v", err)
	}

	server := newTestServer()
	runtime := agent.NewRuntime(database, providers.NewRegistry(), agent.NewTunables(), t.TempDir(), t.TempDir(), nil, nil, "WS1", "Test", nil, server.logger)
	t.Cleanup(runtime.CloseMCP)
	server.runs = newChatRuns()
	server.runs.register("run-worker", worker.ID, "WS1", func() {})
	server.runs.register("run-fork", fork.ID, "WS1", func() {})
	server.runs.register("run-fromidle", fromIdle.ID, "WS1", func() {})
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1", Name: "Test"}, DB: database, Runtime: runtime}

	fetch := func(path string) workspaceGraph {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
		rec := httptest.NewRecorder()
		server.handleWorkspaceGraph(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		var graph workspaceGraph
		if err := json.NewDecoder(rec.Body).Decode(&graph); err != nil {
			t.Fatalf("decode graph: %v", err)
		}
		return graph
	}
	lineage := func(g workspaceGraph) map[string]string {
		out := map[string]string{}
		for _, e := range g.Edges {
			if e.Kind == "spawned" || e.Kind == "forked_from" {
				out[e.Source+">"+e.Target] = e.Kind
			}
		}
		return out
	}

	live := lineage(fetch("/api/graph"))
	want := map[string]string{
		"run:" + coord.ID + ">run:" + worker.ID: "spawned",
		"run:" + worker.ID + ">run:" + fork.ID:  "forked_from",
	}
	if len(live) != len(want) {
		t.Fatalf("live lineage = %v, want %v", live, want)
	}
	for k, v := range want {
		if live[k] != v {
			t.Errorf("live edge %s = %q, want %q (all: %v)", k, live[k], v, live)
		}
	}

	recent := fetch("/api/graph?scope=recent")
	rl := lineage(recent)
	if rl["run:"+idle.ID+">run:"+fromIdle.ID] != "forked_from" {
		t.Errorf("recent scope must draw the idle trigger's fork: %v", rl)
	}
	if len(rl) != 3 {
		t.Errorf("recent lineage = %v, want 3 edges", rl)
	}
	scopeOf := map[string]string{}
	for _, n := range recent.Nodes {
		if n.Type == "run" {
			scopeOf[strings.TrimPrefix(n.ID, "run:")] = n.LiveScope
		}
	}
	if scopeOf[idle.ID] != "recent" || scopeOf[worker.ID] != "running" || scopeOf[coord.ID] != "awaiting-workers" {
		t.Errorf("recent node scopes = %v", scopeOf)
	}
	if recent.Stats["lineage"] != 3 {
		t.Errorf("stats.lineage = %d, want 3", recent.Stats["lineage"])
	}
}
