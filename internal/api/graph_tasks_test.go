package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

func TestWorkspaceGraphHidesArchivedTasks(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	active, err := database.CreateTask(ctx, db.Task{Title: "Active", BoardState: db.BoardTodo})
	if err != nil {
		t.Fatalf("create active task: %v", err)
	}
	flow, err := database.CreateFlow(ctx, db.Flow{Name: "Flow", Graph: `{}`})
	if err != nil {
		t.Fatalf("create flow: %v", err)
	}
	archived, err := database.CreateTask(ctx, db.Task{Title: "Archived", BoardState: db.BoardDone, FlowID: flow.ID})
	if err != nil {
		t.Fatalf("create archived task: %v", err)
	}
	if err := database.SetTaskArchived(ctx, archived.ID, true); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	server := newTestServer()
	runtime := agent.NewRuntime(database, providers.NewRegistry(), agent.NewTunables(), t.TempDir(), t.TempDir(), nil, nil, "WS1", "Test", nil, server.logger)
	t.Cleanup(runtime.CloseMCP)
	server.runs = newChatRuns()
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1", Name: "Test"}, DB: database, Runtime: runtime}
	req := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	server.handleWorkspaceGraph(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var graph workspaceGraph
	if err := json.NewDecoder(rec.Body).Decode(&graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	activeID := "task:" + active.ID
	archivedID := "task:" + archived.ID
	seenActive := false
	for _, node := range graph.Nodes {
		if node.ID == activeID {
			seenActive = true
		}
		if node.ID == archivedID {
			t.Fatalf("archived task node present: %+v", node)
		}
	}
	if !seenActive {
		t.Fatalf("active task node %q missing", activeID)
	}
	for _, edge := range graph.Edges {
		if edge.Source == archivedID || edge.Target == archivedID {
			t.Fatalf("archived task edge present: %+v", edge)
		}
	}
	if got := graph.Stats["tasks"]; got != 1 {
		t.Fatalf("stats.tasks = %d, want 1", got)
	}
}

func TestWorkspaceGraphOnlyEmitsLiveScopeSessionsAndAgents(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	coordinatorAgent, err := database.CreateAgent(ctx, db.Agent{Name: "Coordinator"})
	if err != nil {
		t.Fatalf("create coordinator agent: %v", err)
	}
	workerAgent, err := database.CreateAgent(ctx, db.Agent{Name: "Worker"})
	if err != nil {
		t.Fatalf("create worker agent: %v", err)
	}
	parent, err := database.CreateSession(ctx, db.Session{AgentID: coordinatorAgent.ID, Title: "Parent"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := database.CreateSession(ctx, db.Session{AgentID: workerAgent.ID, Kind: "worker", Title: "Child", CoordinatorSessionID: parent.ID})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	second, err := database.CreateSession(ctx, db.Session{AgentID: workerAgent.ID, Kind: "spawned", Title: "Second"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	idle, err := database.CreateSession(ctx, db.Session{AgentID: workerAgent.ID, Title: "Completed"})
	if err != nil {
		t.Fatalf("create idle: %v", err)
	}
	orphan, err := database.CreateSession(ctx, db.Session{AgentID: "AGT-DELETED", Title: "Orphan"})
	if err != nil {
		t.Fatalf("create orphan: %v", err)
	}

	server := newTestServer()
	runtime := agent.NewRuntime(database, providers.NewRegistry(), agent.NewTunables(), t.TempDir(), t.TempDir(), nil, nil, "WS1", "Test", nil, server.logger)
	t.Cleanup(runtime.CloseMCP)
	server.runs = newChatRuns()
	server.runs.register("run-child", child.ID, "WS1", func() {})
	server.runs.register("run-second", second.ID, "WS1", func() {})
	server.runs.register("run-orphan", orphan.ID, "WS1", func() {})
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1", Name: "Test"}, DB: database, Runtime: runtime}
	req := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
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
	wantScope := map[string]string{parent.ID: "awaiting-workers", child.ID: "running", second.ID: "running"}
	seenRuns := map[string]string{}
	seenAgents := map[string]string{}
	for _, node := range graph.Nodes {
		switch node.Type {
		case "run":
			seenRuns[node.ID[len("run:"):]] = node.LiveScope
		case "agent":
			seenAgents[node.SessionID] = node.LiveScope
		}
		if node.SessionID == idle.ID || node.ID == "run:"+idle.ID || node.SessionID == orphan.ID || node.ID == "run:"+orphan.ID {
			t.Fatalf("idle/orphan node emitted: %+v", node)
		}
	}
	if len(seenRuns) != len(wantScope) || len(seenAgents) != len(wantScope) {
		t.Fatalf("live nodes mismatch: runs=%v agents=%v", seenRuns, seenAgents)
	}
	for id, scope := range wantScope {
		if seenRuns[id] != scope || seenAgents[id] != scope {
			t.Fatalf("session %s scope mismatch: runs=%q agents=%q want=%q", id, seenRuns[id], seenAgents[id], scope)
		}
	}
	if graph.Stats["runs"] != 3 || graph.Stats["agents"] != 3 || graph.Stats["agentsTotal"] != 2 {
		t.Fatalf("filtered stats mismatch: %+v", graph.Stats)
	}
}
