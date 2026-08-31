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
