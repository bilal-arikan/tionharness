package agent

import (
	"context"
	"os"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Flow run GC at the runtime level (_Docs/77 R8): the store removes rows and
// state-delta journals; the runtime owns the per-node step sidecars
// (<store>/flow_runs/<runID>/) and the workspace stream, so deletion goes
// through here.

// DeleteFlowRun removes a run's whole tree — rows, state deltas, step sidecars —
// and announces each deleted run on the workspace stream. ErrNotFound /
// ErrConflict (a live member) propagate from the store untouched so the API can
// map them.
func (r *Runtime) DeleteFlowRun(ctx context.Context, id string) ([]string, error) {
	run, err := r.db.GetFlowRun(ctx, id)
	if err != nil {
		return nil, err
	}
	// Snapshot the tree before deletion so the events carry flow/root ids.
	tree, _ := r.db.ListFlowRunTree(ctx, run.RootOf())
	byID := make(map[string]db.FlowRun, len(tree))
	for _, t := range tree {
		byID[t.ID] = t
	}
	deleted, err := r.db.DeleteFlowRunTree(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, rid := range deleted {
		r.removeFlowRunSidecars(rid)
		if t, ok := byID[rid]; ok {
			t.Status = FlowRunDeleted
			r.emitFlowRunEvent(t)
		}
	}
	return deleted, nil
}

// FlowRunDeleted is the ws:flow_run status announcing a removed run. Not a
// store status — a deleted run has no row.
const FlowRunDeleted = "deleted"

// removeFlowRunSidecars drops a run's per-node step files. Best-effort.
func (r *Runtime) removeFlowRunSidecars(runID string) {
	if runID == "" {
		return
	}
	_ = os.RemoveAll(r.flowRunStepsDir(runID))
}

// sweepFlowRunRetention applies the FlowRunRetention tunable to every flow:
// keep the newest N terminal root runs per flow, delete the rest with their
// trees and sidecars. 0 (the default) keeps everything, so a workspace that
// never set it sees no change.
func (r *Runtime) sweepFlowRunRetention(ctx context.Context) int {
	keep := 0
	if r.tun != nil {
		keep = r.tun.FlowRunRetention()
	}
	if keep <= 0 || r.db == nil {
		return 0
	}
	flows, err := r.db.ListFlows(ctx)
	if err != nil {
		return 0
	}
	total := 0
	for _, f := range flows {
		ids, perr := r.db.PruneFlowRuns(ctx, f.ID, keep)
		if perr != nil {
			r.logger.Warn("flow run retention: prune failed", "flow", f.ID, "error", perr)
			continue
		}
		for _, id := range ids {
			r.removeFlowRunSidecars(id)
		}
		total += len(ids)
	}
	if total > 0 {
		r.logger.Info("flow run retention: pruned", "runs", total, "keepPerFlow", keep)
	}
	return total
}
