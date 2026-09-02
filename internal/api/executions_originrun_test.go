package api

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestLastStatusForUsesTheSessionsOwnRun: a flow session linked to its run
// (Origin.RunID, R1) reports THAT run's status even when the flow has a newer
// run; a pre-link session still falls back to the flow's newest run.
func TestLastStatusForUsesTheSessionsOwnRun(t *testing.T) {
	ctx := context.Background()
	s, wsp := newWorkspaceServer(t)
	flow, err := wsp.DB.CreateFlow(ctx, db.Flow{Name: "f"})
	if err != nil {
		t.Fatalf("CreateFlow: %v", err)
	}
	old, _ := wsp.DB.CreateFlowRun(ctx, db.FlowRun{FlowID: flow.ID})
	if err := wsp.DB.FinishFlowRun(ctx, old.ID, db.FlowFailure, "", "boom"); err != nil {
		t.Fatalf("finish old: %v", err)
	}
	newer, _ := wsp.DB.CreateFlowRun(ctx, db.FlowRun{FlowID: flow.ID})
	if err := wsp.DB.FinishFlowRun(ctx, newer.ID, db.FlowSuccess, "", ""); err != nil {
		t.Fatalf("finish newer: %v", err)
	}
	runs, _ := wsp.DB.ListFlowRuns(ctx, "")
	flowStatus := newestFlowRunStatus(runs)
	runStatus := flowRunStatusByID(runs)

	linked := db.Session{Kind: "flow", SourceID: flow.ID, Origin: &db.SessionOrigin{Kind: db.OriginFlow, EntityID: flow.ID, RunID: old.ID}}
	if got := s.lastStatusFor(ctx, wsp, linked, flowStatus, runStatus); got != db.FlowFailure {
		t.Fatalf("linked session status = %q, want its own run's %q (not the flow's newest)", got, db.FlowFailure)
	}
	legacy := db.Session{Kind: "flow", SourceID: flow.ID}
	if got := s.lastStatusFor(ctx, wsp, legacy, flowStatus, runStatus); got != db.FlowSuccess {
		t.Fatalf("legacy session status = %q, want the flow's newest %q", got, db.FlowSuccess)
	}
	// A linked session whose run was deleted (retention) falls back too.
	orphan := db.Session{Kind: "flow", SourceID: flow.ID, Origin: &db.SessionOrigin{Kind: db.OriginFlow, EntityID: flow.ID, RunID: "RUN-gone"}}
	if got := s.lastStatusFor(ctx, wsp, orphan, flowStatus, runStatus); got != db.FlowSuccess {
		t.Fatalf("orphaned session status = %q, want fallback %q", got, db.FlowSuccess)
	}
}
