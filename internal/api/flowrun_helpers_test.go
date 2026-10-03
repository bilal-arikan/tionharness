package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// seedFlowRun creates an agent (when the store has none) with its default flow
// and opens one run of it. status "" leaves the run running; FlowSuccess /
// FlowFailure close it right away.
func seedFlowRun(tb testing.TB, database *db.DB, status, errText string) db.FlowRun {
	tb.Helper()
	ctx := context.Background()
	agents, _ := database.ListAgents(ctx)
	var agentID string
	if len(agents) == 0 {
		a, err := database.CreateAgent(ctx, db.Agent{Name: "flow-agent", Provider: "anthropic", Model: "m"})
		if err != nil {
			tb.Fatalf("create agent: %v", err)
		}
		agentID = a.ID
	} else {
		agentID = agents[0].ID
	}
	flow, err := database.EnsureAgentFlow(ctx, agentID)
	if err != nil {
		tb.Fatalf("ensure flow: %v", err)
	}
	run, err := database.CreateFlowRun(ctx, db.FlowRun{FlowID: flow.ID, Input: "x"})
	if err != nil {
		tb.Fatalf("create run: %v", err)
	}
	if status != "" {
		run, err = database.FinishFlowRun(ctx, run.ID, status, "", errText, nil, 0, 0, db.FlowRunUsage{})
		if err != nil {
			tb.Fatalf("finish run: %v", err)
		}
	}
	return run
}

// serveFlowRuns drives a handler against a bare workspace wrapping database.
func serveFlowRuns(h http.HandlerFunc, database *db.DB, target string, pathValues map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	ctx := context.WithValue(req.Context(), workspaceCtxKey, &workspace.Workspace{DB: database})
	rec := httptest.NewRecorder()
	h(rec, req.WithContext(ctx))
	return rec
}
