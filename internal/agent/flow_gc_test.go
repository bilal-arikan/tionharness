package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// TestDeleteFlowRunRemovesSidecarsAndAnnounces: the runtime-level delete drops
// the per-node step sidecar directory and publishes ws:flow_run deleted for
// every removed member; the retention sweep does the same for old trees.
func TestDeleteFlowRunRemovesSidecarsAndAnnounces(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	rt.wsID = "WS-test"
	drain := collectWS(t, rt)
	ctx := context.Background()
	flow, err := rt.db.CreateFlow(ctx, db.Flow{Name: "f", Graph: `{"start":"s","nodes":[{"id":"s","type":"start"}]}`})
	if err != nil {
		t.Fatalf("create flow: %v", err)
	}
	mkRun := func() db.FlowRun {
		run, _ := rt.db.CreateFlowRun(ctx, db.FlowRun{FlowID: flow.ID})
		if err := rt.db.FinishFlowRun(ctx, run.ID, db.FlowSuccess, "", ""); err != nil {
			t.Fatalf("finish: %v", err)
		}
		dir := rt.flowRunStepsDir(run.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "steps-n1.json"), []byte("[]"), 0o644); err != nil {
			t.Fatalf("write sidecar: %v", err)
		}
		return run
	}
	old := mkRun()
	newer := mkRun()
	// Make ordering deterministic: old is older.
	rt.db.SetFlowRunCreatedAtForTest(old.ID, 1000)
	rt.db.SetFlowRunCreatedAtForTest(newer.ID, 2000)

	deleted, err := rt.DeleteFlowRun(ctx, old.ID)
	if err != nil || len(deleted) != 1 || deleted[0] != old.ID {
		t.Fatalf("delete = %v err=%v", deleted, err)
	}
	if _, err := os.Stat(rt.flowRunStepsDir(old.ID)); !os.IsNotExist(err) {
		t.Fatal("step sidecar dir must be removed with the run")
	}
	if _, err := os.Stat(rt.flowRunStepsDir(newer.ID)); err != nil {
		t.Fatal("the other run's sidecars must be untouched")
	}
	var announced []FlowRunPayload
	for _, e := range drain() {
		if e.Type == events.TypeWSFlowRun {
			p := decodeData[FlowRunPayload](t, e)
			if p.Status == FlowRunDeleted {
				announced = append(announced, p)
			}
		}
	}
	if len(announced) != 1 || announced[0].RunID != old.ID {
		t.Fatalf("deleted announcements = %+v, want one for %s", announced, old.ID)
	}

	// Retention: keep 1 per flow → with a third finished run, the oldest goes.
	third := mkRun()
	rt.db.SetFlowRunCreatedAtForTest(third.ID, 3000)
	tun.SetFlowRunRetention(1)
	if n := rt.sweepFlowRunRetention(ctx); n != 1 {
		t.Fatalf("sweep pruned %d, want 1", n)
	}
	if _, err := rt.db.GetFlowRun(ctx, newer.ID); err == nil {
		t.Fatal("older finished run must be pruned by retention")
	}
	if _, err := os.Stat(rt.flowRunStepsDir(newer.ID)); !os.IsNotExist(err) {
		t.Fatal("pruned run's sidecars must be removed")
	}
	if _, err := rt.db.GetFlowRun(ctx, third.ID); err != nil {
		t.Fatal("newest finished run must survive")
	}
	tun.SetFlowRunRetention(0)
	if n := rt.sweepFlowRunRetention(ctx); n != 0 {
		t.Fatalf("retention 0 must prune nothing, pruned %d", n)
	}
}
