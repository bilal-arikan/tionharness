package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func gcStore(t *testing.T) (*DB, string) {
	t.Helper()
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	f, err := d.CreateFlow(ctx, Flow{Name: "f", Graph: `{"start":"s","nodes":[{"id":"s","type":"start"}]}`})
	if err != nil {
		t.Fatalf("create flow: %v", err)
	}
	return d, f.ID
}

// TestDeleteFlowRunTreeRefusesLiveMembers: a tree with a running or waiting
// member is protected; once finished, deleting any member removes the whole
// tree (rows + state deltas).
func TestDeleteFlowRunTreeRefusesLiveMembers(t *testing.T) {
	ctx := context.Background()
	d, flowID := gcStore(t)
	root, _ := d.CreateFlowRun(ctx, FlowRun{FlowID: flowID})
	child, _ := d.CreateFlowRun(ctx, FlowRun{FlowID: flowID, ParentRunID: root.ID, ParentNodeID: "n1", RootRunID: root.ID})
	if err := d.FinishFlowRun(ctx, root.ID, FlowSuccess, "ok", ""); err != nil {
		t.Fatalf("finish root: %v", err)
	}
	// Child still running → the tree is live, whichever member is named.
	if _, err := d.DeleteFlowRunTree(ctx, root.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete with running child: err=%v, want ErrConflict", err)
	}
	if err := d.MarkFlowRunWaiting(ctx, child.ID, `{}`); err != nil {
		t.Fatalf("mark waiting: %v", err)
	}
	if _, err := d.DeleteFlowRunTree(ctx, child.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete with waiting child: err=%v, want ErrConflict", err)
	}
	if _, err := d.ClaimWaitingFlowRun(ctx, child.ID); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := d.FinishFlowRun(ctx, child.ID, FlowFailure, "", "boom"); err != nil {
		t.Fatalf("finish child: %v", err)
	}
	if err := d.AppendFlowRunStateDelta(ctx, child.ID, FlowRunStateDelta{Version: FlowRunStateDeltaVersion, Sequence: 1}); err != nil {
		t.Logf("delta append (optional): %v", err)
	}
	deleted, err := d.DeleteFlowRunTree(ctx, child.ID) // naming the child deletes the tree
	if err != nil || len(deleted) != 2 {
		t.Fatalf("delete tree = %v err=%v, want both runs", deleted, err)
	}
	if _, err := d.GetFlowRun(ctx, root.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("root must be gone")
	}
	if _, err := d.DeleteFlowRunTree(ctx, "RUN-nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	if d.runningFlowRuns.Load() != 0 {
		t.Fatalf("running counter drifted to %d", d.runningFlowRuns.Load())
	}
}

// TestPruneFlowRunsKeepsNewestTerminalRoots: retention keeps the newest N
// finished root trees per flow, never a live tree, and 0 keeps everything.
func TestPruneFlowRunsKeepsNewestTerminalRoots(t *testing.T) {
	ctx := context.Background()
	d, flowID := gcStore(t)
	var roots []FlowRun
	for i := 0; i < 5; i++ {
		r, _ := d.CreateFlowRun(ctx, FlowRun{FlowID: flowID})
		roots = append(roots, r)
	}
	// Give each root a distinct createdAt so ordering is deterministic.
	d.mu.Lock()
	for i, r := range roots {
		cur := d.flowRuns[r.ID]
		cur.CreatedAt = int64(1000 + i)
		d.flowRuns[r.ID] = cur
	}
	d.mu.Unlock()
	// roots[0..3] finished, roots[4] (newest) still running.
	for _, r := range roots[:4] {
		if err := d.FinishFlowRun(ctx, r.ID, FlowSuccess, "", ""); err != nil {
			t.Fatalf("finish: %v", err)
		}
	}
	if ids, err := d.PruneFlowRuns(ctx, flowID, 0); err != nil || len(ids) != 0 {
		t.Fatalf("keep=0 must be a no-op, got %v err=%v", ids, err)
	}
	ids, err := d.PruneFlowRuns(ctx, flowID, 2)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	// Terminal roots newest-first: roots[3], roots[2] kept; roots[1], roots[0] pruned.
	if len(ids) != 2 {
		t.Fatalf("pruned %v, want the two oldest finished roots", ids)
	}
	for _, keep := range []string{roots[2].ID, roots[3].ID, roots[4].ID} {
		if _, err := d.GetFlowRun(ctx, keep); err != nil {
			t.Fatalf("run %s must survive: %v", keep, err)
		}
	}
	for _, gone := range []string{roots[0].ID, roots[1].ID} {
		if _, err := d.GetFlowRun(ctx, gone); !errors.Is(err, ErrNotFound) {
			t.Fatalf("run %s must be pruned", gone)
		}
	}
}
