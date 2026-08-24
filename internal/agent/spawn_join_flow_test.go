package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

// TestSpawnJoin_Integration exercises the real runtime wiring: a parent flow
// spawns two transform-only child flows (no LLM) and a join barrier block-polls
// them to completion, collecting their outputs. Proves spawnChildFlow +
// JoinChildFlows + real driveFlow/DB work end-to-end.
func TestSpawnJoin_Integration(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()

	child := createFlow(t, rt, orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "t"},
			{ID: "t", Type: orchestration.NodeTransform, Template: "child:{{input}}", Next: "end"},
			{ID: "end", Type: orchestration.NodeEnd},
		},
	})

	parent := createFlow(t, rt, orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "sp"},
			{ID: "sp", Type: orchestration.NodeSpawn, SpawnFlows: []string{child, child}, Template: "{{input}}", Next: "jn"},
			{ID: "jn", Type: orchestration.NodeJoin, SpawnRef: "sp", Next: "end"},
			{ID: "end", Type: orchestration.NodeEnd},
		},
	})

	run, err := rt.RunFlow(ctx, parent, "go", false, nil)
	if err != nil {
		t.Fatalf("parent run failed: %v", err)
	}
	if run.Status != db.FlowSuccess {
		t.Fatalf("parent should succeed, got %q (%s)", run.Status, run.Error)
	}
	if run.Output != "child:go\n\nchild:go" {
		t.Errorf("join should collect both child outputs, got %q", run.Output)
	}
}

// TestJoin_PartialDropsFailedChild proves the partial-mode barrier: one child
// succeeds, one fails at runtime (its end node's OutputSchema rejects a non-JSON
// output). With JoinPartial the join drops the failure and returns the success;
// strict mode fails the whole join.
func TestJoin_PartialDropsFailedChild(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()

	childOK := createFlow(t, rt, orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "t"},
			{ID: "t", Type: orchestration.NodeTransform, Template: "ok", Next: "end"},
			{ID: "end", Type: orchestration.NodeEnd},
		},
	})
	// childFail's end node requires JSON but the output is "nope" → run fails.
	childFail := createFlow(t, rt, orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "t"},
			{ID: "t", Type: orchestration.NodeTransform, Template: "nope", Next: "end"},
			{ID: "end", Type: orchestration.NodeEnd, OutputSchema: `{"type":"object"}`},
		},
	})

	mkParent := func(partial bool) string {
		return createFlow(t, rt, orchestration.Graph{
			Start: "start",
			Nodes: []orchestration.Node{
				{ID: "start", Type: orchestration.NodeStart, Next: "sp"},
				{ID: "sp", Type: orchestration.NodeSpawn, SpawnFlows: []string{childOK, childFail}, Next: "jn"},
				{ID: "jn", Type: orchestration.NodeJoin, SpawnRef: "sp", JoinPartial: partial, Next: "end"},
				{ID: "end", Type: orchestration.NodeEnd},
			},
		})
	}

	// Partial: drops the failed child, returns only the success.
	run, err := rt.RunFlow(ctx, mkParent(true), "go", false, nil)
	if err != nil {
		t.Fatalf("partial parent run failed: %v", err)
	}
	if run.Status != db.FlowSuccess || run.Output != "ok" {
		t.Fatalf("partial join should drop the failure and return \"ok\", got %q (%s)", run.Output, run.Status)
	}

	// Strict: the failed child fails the whole join.
	strict, err := rt.RunFlow(ctx, mkParent(false), "go", false, nil)
	if err != nil {
		t.Fatalf("strict parent run returned a setup error: %v", err)
	}
	if strict.Status != db.FlowFailure {
		t.Errorf("strict join should fail when a child fails, got %q", strict.Status)
	}
}

// TestSubflowAwaitPropagation_Integration proves the full cross-flow suspend/
// resume path: a parent's subflow child suspends at await-input → the PARENT run
// goes to status=waiting; feeding the parent resumes the child, which completes,
// and the parent finishes with the child's output.
func TestSubflowAwaitPropagation_Integration(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()

	child := createFlow(t, rt, orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "w"},
			{ID: "w", Type: orchestration.NodeAwaitInput, Next: "t"},
			{ID: "t", Type: orchestration.NodeTransform, Template: "resumed:{{last}}", Next: "end"},
			{ID: "end", Type: orchestration.NodeEnd},
		},
	})

	parent := createFlow(t, rt, orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "sub"},
			{ID: "sub", Type: orchestration.NodeSubflow, FlowRef: child, Template: "{{input}}", Next: "end"},
			{ID: "end", Type: orchestration.NodeEnd},
		},
	})

	// Phase 1: parent parks because the child suspends at await-input.
	run, err := rt.RunFlow(ctx, parent, "hi", false, nil)
	if err != nil {
		t.Fatalf("parent run failed: %v", err)
	}
	if run.Status != db.FlowWaiting {
		t.Fatalf("parent should be WAITING (child suspended), got %q", run.Status)
	}

	// Phase 2: feed the parent → child resumes → parent completes.
	if _, err := rt.ResumeWaitingFlow(ctx, run.ID, "answer"); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	final := waitFlowTerminal(t, rt, run.ID)
	if final.Status != db.FlowSuccess {
		t.Fatalf("parent should succeed after resume, got %q (%s)", final.Status, final.Error)
	}
	if !strings.Contains(final.Output, "resumed:answer") {
		t.Errorf("parent output should carry the resumed child result, got %q", final.Output)
	}
}

// waitFlowTerminal polls a run until it leaves running/waiting or a short timeout.
func waitFlowTerminal(t *testing.T, rt *Runtime, runID string) db.FlowRun {
	t.Helper()
	for i := 0; i < 200; i++ {
		r, err := rt.db.GetFlowRun(context.Background(), runID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if r.Status == db.FlowSuccess || r.Status == db.FlowFailure {
			return r
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach a terminal status in time", runID)
	return db.FlowRun{}
}
