package db

import (
	"context"
	"math/rand"
	"testing"
)

// openRunCountTestDB opens a throwaway store for the counter tests.
func openRunCountTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return d
}

// scanRunningFlowRuns is the brute-force ground truth every counter assertion
// compares against — deliberately the naive scan the counter replaced.
func scanRunningFlowRuns(d *DB) int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var n int64
	for _, r := range d.flowRuns {
		if r.Status == FlowRunning {
			n++
		}
	}
	return n
}

// assertFlowCounter fails when the O(1) counter disagrees with a full scan.
func assertFlowCounter(t *testing.T, d *DB, step string, want int64) {
	t.Helper()
	if got := d.runningFlowRuns.Load(); got != want {
		t.Fatalf("%s: runningFlowRuns = %d, want %d", step, got, want)
	}
	if got := scanRunningFlowRuns(d); got != want {
		t.Fatalf("%s: scan = %d, want %d (counter and store disagree)", step, got, want)
	}
	if got := d.HasRunningFlowRuns(); got != (want > 0) {
		t.Fatalf("%s: HasRunningFlowRuns = %v, want %v", step, got, want > 0)
	}
}

// TestFlowRunCounterLifecycle walks a run through every status transition the
// store exposes and asserts the counter tracks each edge.
func TestFlowRunCounterLifecycle(t *testing.T) {
	ctx := context.Background()
	d := openRunCountTestDB(t)

	assertFlowCounter(t, d, "empty store", 0)

	run, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW1", Input: "hi"})
	if err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}
	assertFlowCounter(t, d, "after create", 1)

	// Non-status writes must not move the counter.
	if err := d.SetFlowRunState(ctx, run.ID, `{"a":1}`); err != nil {
		t.Fatalf("SetFlowRunState: %v", err)
	}
	assertFlowCounter(t, d, "after SetFlowRunState", 1)

	if err := d.SetFlowRunSession(ctx, run.ID, "SES1"); err != nil {
		t.Fatalf("SetFlowRunSession: %v", err)
	}
	assertFlowCounter(t, d, "after SetFlowRunSession", 1)

	// running → waiting releases the slot: a suspended run is not running.
	if err := d.MarkFlowRunWaiting(ctx, run.ID, `{"b":2}`); err != nil {
		t.Fatalf("MarkFlowRunWaiting: %v", err)
	}
	assertFlowCounter(t, d, "after MarkFlowRunWaiting", 0)

	// waiting → running takes it back.
	if _, err := d.ClaimWaitingFlowRun(ctx, run.ID); err != nil {
		t.Fatalf("ClaimWaitingFlowRun: %v", err)
	}
	assertFlowCounter(t, d, "after ClaimWaitingFlowRun", 1)

	if err := d.FinishFlowRun(ctx, run.ID, FlowSuccess, "out", ""); err != nil {
		t.Fatalf("FinishFlowRun: %v", err)
	}
	assertFlowCounter(t, d, "after FinishFlowRun", 0)

	// Finishing an already-terminal run must not double-decrement.
	if err := d.FinishFlowRun(ctx, run.ID, FlowFailure, "", "boom"); err != nil {
		t.Fatalf("FinishFlowRun (again): %v", err)
	}
	assertFlowCounter(t, d, "after second FinishFlowRun", 0)
}

// TestFlowRunCounterDeleteFlowReleasesRunning locks the cascade path: deleting a
// flow drops its runs, and the running ones must give their slots back.
func TestFlowRunCounterDeleteFlowReleasesRunning(t *testing.T) {
	ctx := context.Background()
	d := openRunCountTestDB(t)

	flow, err := d.CreateFlow(ctx, Flow{Name: "f"})
	if err != nil {
		t.Fatalf("CreateFlow: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := d.CreateFlowRun(ctx, FlowRun{FlowID: flow.ID}); err != nil {
			t.Fatalf("CreateFlowRun: %v", err)
		}
	}
	done, err := d.CreateFlowRun(ctx, FlowRun{FlowID: flow.ID})
	if err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}
	if err := d.FinishFlowRun(ctx, done.ID, FlowSuccess, "", ""); err != nil {
		t.Fatalf("FinishFlowRun: %v", err)
	}
	assertFlowCounter(t, d, "before delete", 2)

	if err := d.DeleteFlow(ctx, flow.ID); err != nil {
		t.Fatalf("DeleteFlow: %v", err)
	}
	assertFlowCounter(t, d, "after DeleteFlow", 0)
}

// TestFlowRunCounterSurvivesReload proves load() seeds the counter from disk —
// otherwise every restart would report an idle workspace as busy (or vice versa).
func TestFlowRunCounterSurvivesReload(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	d, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	running, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW1"})
	if err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}
	waiting, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW1"})
	if err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}
	if err := d.MarkFlowRunWaiting(ctx, waiting.ID, "{}"); err != nil {
		t.Fatalf("MarkFlowRunWaiting: %v", err)
	}
	finished, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW1"})
	if err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}
	if err := d.FinishFlowRun(ctx, finished.ID, FlowSuccess, "", ""); err != nil {
		t.Fatalf("FinishFlowRun: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	// Only `running` counts: waiting runs sleep until input, finished ones are done.
	assertFlowCounter(t, reopened, "after reload", 1)
	if _, err := reopened.GetFlowRun(ctx, running.ID); err != nil {
		t.Fatalf("running run missing after reload: %v", err)
	}
}

// TestFlowRunCounterRandomizedOps hammers a deterministic pseudo-random sequence
// of transitions and asserts the counter never drifts from the store. A fixed
// seed keeps it reproducible under -race in CI.
func TestFlowRunCounterRandomizedOps(t *testing.T) {
	ctx := context.Background()
	d := openRunCountTestDB(t)
	rng := rand.New(rand.NewSource(1337))

	var ids []string
	for i := 0; i < 500; i++ {
		switch rng.Intn(5) {
		case 0:
			r, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW1"})
			if err != nil {
				t.Fatalf("op %d CreateFlowRun: %v", i, err)
			}
			ids = append(ids, r.ID)
		case 1:
			if len(ids) == 0 {
				continue
			}
			// Errors are expected here (wrong current status) and are not failures:
			// the point is that a REJECTED transition must not move the counter.
			_ = d.MarkFlowRunWaiting(ctx, ids[rng.Intn(len(ids))], "{}")
		case 2:
			if len(ids) == 0 {
				continue
			}
			_, _ = d.ClaimWaitingFlowRun(ctx, ids[rng.Intn(len(ids))])
		case 3:
			if len(ids) == 0 {
				continue
			}
			_ = d.FinishFlowRun(ctx, ids[rng.Intn(len(ids))], FlowSuccess, "", "")
		case 4:
			if len(ids) == 0 {
				continue
			}
			_ = d.SetFlowRunState(ctx, ids[rng.Intn(len(ids))], "{}")
		}

		if got, want := d.runningFlowRuns.Load(), scanRunningFlowRuns(d); got != want {
			t.Fatalf("op %d: counter = %d, scan = %d", i, got, want)
		}
	}
}

// TestReconcileDetectsDrift pokes the map behind the counter's back (what a
// future status-flipping path that forgot the counter would effectively do) and
// asserts the guard both REPORTS and repairs it. Reporting is the point: a
// silent self-heal would hide the underlying bug forever.
func TestReconcileDetectsDrift(t *testing.T) {
	ctx := context.Background()
	d := openRunCountTestDB(t)

	if _, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW1"}); err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}
	if drift := d.ReconcileRunCounters(); !drift.Zero() {
		t.Fatalf("healthy store reported drift: %+v", drift)
	}

	// Bypass the store API entirely: two running rows the counter never saw.
	d.mu.Lock()
	d.flowRuns["ghost1"] = FlowRun{ID: "ghost1", FlowID: "FLW1", Status: FlowRunning}
	d.flowRuns["ghost2"] = FlowRun{ID: "ghost2", FlowID: "FLW1", Status: FlowRunning}
	d.mu.Unlock()

	drift := d.ReconcileRunCounters()
	if drift.FlowRuns != -2 {
		t.Fatalf("flow-run drift = %d, want -2 (counter under-counted by two)", drift.FlowRuns)
	}
	if drift.Zero() {
		t.Fatal("Zero() reported healthy on a drifted store")
	}

	// Repaired: a second pass finds nothing and the counters match the scan.
	if again := d.ReconcileRunCounters(); !again.Zero() {
		t.Fatalf("drift survived reconcile: %+v", again)
	}
	if got, want := d.runningFlowRuns.Load(), scanRunningFlowRuns(d); got != want {
		t.Fatalf("after repair: counter = %d, scan = %d", got, want)
	}
}
