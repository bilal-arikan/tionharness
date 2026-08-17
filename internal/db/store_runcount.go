package db

// This file holds the drift guard for the O(1) running-flow-run counter declared
// in db.go. The counter is maintained incrementally by the status-transition
// paths (persistFlowRunLocked / deleteFlowRunLocked); this is the periodic check
// that those paths did not miss an edge.

// RunCounterDrift is how far the counter has strayed from a ground-truth scan,
// expressed as counted - actual. Zero is the only healthy value.
type RunCounterDrift struct {
	FlowRuns int64
}

// Zero reports whether the counter agrees with the store.
func (d RunCounterDrift) Zero() bool { return d.FlowRuns == 0 }

// ReconcileRunCounters recomputes the running-flow-run counter from the map and
// resets it to the scanned truth, returning the drift it found.
//
// A NON-ZERO result is a BUG, not a normal condition: it means some path flipped
// a run's status without going through the counter. Callers must surface it —
// the correction here keeps the UI honest, but swallowing the signal would let
// the real defect live on invisibly.
//
// The scan is exactly what the counter exists to avoid, so this must only ever
// run on a slow path (boot, and the low-frequency sweeper tick).
func (d *DB) ReconcileRunCounters() RunCounterDrift {
	d.mu.RLock()
	var flowRuns int64
	for _, r := range d.flowRuns {
		if r.Status == FlowRunning {
			flowRuns++
		}
	}
	d.mu.RUnlock()

	return RunCounterDrift{FlowRuns: d.runningFlowRuns.Swap(flowRuns) - flowRuns}
}
