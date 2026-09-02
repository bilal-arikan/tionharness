package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTrajStore(t *testing.T) (*DB, string, string) {
	t.Helper()
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	root, _ := d.CreateSession(ctx, Session{AgentID: ag.ID, Title: "root", CoordinatorMode: true})
	return d, dir, root.ID
}

func planDevTest(root string) Trajectory {
	return Trajectory{
		RootSessionID: root,
		TemplateRef:   "coordinator-wf-plan-dev-test@3",
		Nodes: []TrajectoryNode{
			{ID: "p:plan", Kind: TrajNodePhase, Origin: TrajOriginDeclared, Label: "plan", State: TrajStatePending, Profile: "planner"},
			{ID: "p:code", Kind: TrajNodePhase, Origin: TrajOriginDeclared, Label: "kod", State: TrajStatePending, Profile: "coder"},
			{ID: "s:" + root, Kind: TrajNodeSession, Origin: TrajOriginObserved, RefKind: "session", RefID: root, State: TrajStateActive},
			{ID: "a:AUT4", Kind: TrajNodeAutomation, Origin: TrajOriginDeclared, RefKind: "automation", RefID: "AUT4", PhaseID: "p:code", Lane: 1, State: TrajStateGhost},
		},
		Edges: []TrajectoryEdge{
			{From: "p:plan", To: "p:code", Kind: TrajEdgeNext, Origin: TrajOriginDeclared},
		},
	}
}

// TestTrajectoryCreateGetList covers the happy path: ids, defaults, sidecar
// location, index listing and the one-per-root rule.
func TestTrajectoryCreateGetList(t *testing.T) {
	ctx := context.Background()
	d, dir, root := newTrajStore(t)

	tr, err := d.CreateTrajectory(ctx, planDevTest(root))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tr.ID[:3] != "RTA" || tr.Revision != 1 || tr.Status != TrajStatusPlanned || tr.CreatedAt == 0 {
		t.Fatalf("created = %+v, want RTA id, revision 1, planned, timestamps", tr)
	}
	if _, err := os.Stat(filepath.Join(dir, dirSessions, root, trajectoryFile)); err != nil {
		t.Fatalf("sidecar must live in the root session's directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, dirTrajectories, trajectoryIndexFile)); err != nil {
		t.Fatalf("index must be written on create: %v", err)
	}

	got, err := d.GetTrajectory(ctx, tr.ID)
	if err != nil || got.ID != tr.ID || len(got.Nodes) != 4 || len(got.Edges) != 1 {
		t.Fatalf("get = %+v err=%v", got, err)
	}
	byRoot, err := d.GetTrajectoryByRoot(ctx, root)
	if err != nil || byRoot.ID != tr.ID {
		t.Fatalf("get by root = %+v err=%v", byRoot, err)
	}
	if _, err := d.GetTrajectory(ctx, "RTA-nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: err=%v, want ErrNotFound", err)
	}

	rows := d.ListTrajectories(ctx, TrajectoryFilter{})
	if len(rows) != 1 || rows[0].ID != tr.ID || rows[0].NodeCount != 4 || rows[0].TemplateRef != tr.TemplateRef {
		t.Fatalf("list = %+v", rows)
	}
	live := false
	if got := d.ListTrajectories(ctx, TrajectoryFilter{Terminal: &live}); len(got) != 1 {
		t.Fatalf("live filter = %+v, want the planned row", got)
	}
	if got := d.ListTrajectories(ctx, TrajectoryFilter{Status: TrajStatusDone}); len(got) != 0 {
		t.Fatalf("status filter = %+v, want none", got)
	}

	// One trajectory per root.
	if _, err := d.CreateTrajectory(ctx, planDevTest(root)); !errors.Is(err, ErrConflict) {
		t.Fatalf("second create for the same root: err=%v, want ErrConflict", err)
	}
	// Root must exist.
	if _, err := d.CreateTrajectory(ctx, Trajectory{RootSessionID: "SES-ghost"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing root: err=%v, want ErrNotFound", err)
	}
}

// TestTrajectoryValidation: structurally invalid graphs are rejected unwritten,
// on create and on update alike.
func TestTrajectoryValidation(t *testing.T) {
	ctx := context.Background()
	d, _, root := newTrajStore(t)

	bad := planDevTest(root)
	bad.Nodes = append(bad.Nodes, TrajectoryNode{ID: "x", Kind: "martian", Origin: TrajOriginDeclared, State: TrajStatePending})
	if _, err := d.CreateTrajectory(ctx, bad); err == nil {
		t.Fatal("unknown node kind must be rejected")
	}
	if d.trajectorySidecar(root).Exists() {
		t.Fatal("a rejected create must not leave a sidecar behind")
	}

	tr, err := d.CreateTrajectory(ctx, planDevTest(root))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = d.UpdateTrajectory(ctx, tr.ID, 0, func(x *Trajectory) error {
		x.Edges = append(x.Edges, TrajectoryEdge{From: "p:plan", To: "nowhere", Kind: TrajEdgeNext, Origin: TrajOriginDeclared})
		return nil
	})
	if err == nil {
		t.Fatal("dangling edge must be rejected")
	}
	_, err = d.UpdateTrajectory(ctx, tr.ID, 0, func(x *Trajectory) error {
		x.Nodes = append(x.Nodes, TrajectoryNode{ID: "s:w", Kind: TrajNodeSession, Origin: TrajOriginObserved, PhaseID: "s:" + root, State: TrajStateActive})
		return nil
	})
	if err == nil {
		t.Fatal("phaseId pointing at a non-phase node must be rejected")
	}
	after, _ := d.GetTrajectory(ctx, tr.ID)
	if after.Revision != 1 || len(after.Edges) != 1 {
		t.Fatalf("rejected updates must not persist: %+v", after)
	}
}

// TestTrajectoryUpdateCAS: an observer append (expectedRev 0) always lands and
// bumps the revision; a UI/agent edit on a stale revision is refused with
// ErrConflict; fn's error aborts without a write; identity fields are protected.
func TestTrajectoryUpdateCAS(t *testing.T) {
	ctx := context.Background()
	d, _, root := newTrajStore(t)
	tr, _ := d.CreateTrajectory(ctx, planDevTest(root))

	var ops []string
	d.SetTrajectoryHook(func(ev TrajectoryChangeEvent) { ops = append(ops, ev.Op) })

	// Observer append on whatever revision is current.
	obs, err := d.UpdateTrajectory(ctx, tr.ID, 0, func(x *Trajectory) error {
		x.Nodes = append(x.Nodes, TrajectoryNode{ID: "s:SES9", Kind: TrajNodeSession, Origin: TrajOriginObserved, RefKind: "session", RefID: "SES9", PhaseID: "p:plan", Lane: 1, State: TrajStateActive})
		x.Edges = append(x.Edges, TrajectoryEdge{From: "s:" + root, To: "s:SES9", Kind: TrajEdgeSpawned, Origin: TrajOriginObserved})
		x.Status = TrajStatusRunning
		return nil
	})
	if err != nil || obs.Revision != 2 {
		t.Fatalf("observer update: rev=%d err=%v, want rev 2", obs.Revision, err)
	}

	// Stale UI edit: rendered revision 1, store is at 2.
	_, err = d.UpdateTrajectory(ctx, tr.ID, 1, func(x *Trajectory) error {
		x.Nodes[0].State = TrajStateSkipped
		return nil
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale CAS: err=%v, want ErrConflict", err)
	}
	// Fresh UI edit on revision 2 succeeds.
	ui, err := d.UpdateTrajectory(ctx, tr.ID, 2, func(x *Trajectory) error {
		x.Nodes[0].State = TrajStateDone
		x.ID = "RTA-forged"
		x.RootSessionID = "SES-forged"
		return nil
	})
	if err != nil || ui.Revision != 3 || ui.Nodes[0].State != TrajStateDone {
		t.Fatalf("fresh CAS: %+v err=%v", ui, err)
	}
	if ui.ID != tr.ID || ui.RootSessionID != root {
		t.Fatalf("identity fields must be protected from fn: id=%s root=%s", ui.ID, ui.RootSessionID)
	}
	// fn error aborts, no write, no hook.
	abort := errors.New("nope")
	if _, err := d.UpdateTrajectory(ctx, tr.ID, 0, func(*Trajectory) error { return abort }); !errors.Is(err, abort) {
		t.Fatalf("fn error must propagate, got %v", err)
	}
	cur, _ := d.GetTrajectory(ctx, tr.ID)
	if cur.Revision != 3 {
		t.Fatalf("aborted update must not bump the revision, got %d", cur.Revision)
	}
	if rows := d.ListTrajectories(ctx, TrajectoryFilter{}); rows[0].Revision != 3 || rows[0].Status != TrajStatusRunning || rows[0].NodeCount != 5 {
		t.Fatalf("index must track the latest write: %+v", rows[0])
	}
	if len(ops) != 2 || ops[0] != TrajectoryOpUpdate || ops[1] != TrajectoryOpUpdate {
		t.Fatalf("hook ops = %v, want two updates (no event for the refused/aborted ones)", ops)
	}
	if _, err := d.UpdateTrajectory(ctx, "RTA-nope", 0, func(*Trajectory) error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: err=%v, want ErrNotFound", err)
	}
}

// TestTrajectoryIndexSurvivesReopenAndRebuild: the index reloads at boot; a
// corrupt index is quarantined and rebuilt from the sidecars.
func TestTrajectoryIndexSurvivesReopenAndRebuild(t *testing.T) {
	ctx := context.Background()
	d, dir, root := newTrajStore(t)
	tr, _ := d.CreateTrajectory(ctx, planDevTest(root))
	_ = d.Close()

	d2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if rows := d2.ListTrajectories(ctx, TrajectoryFilter{RootSessionID: root}); len(rows) != 1 || rows[0].ID != tr.ID {
		t.Fatalf("index after reopen = %+v", rows)
	}
	if got, err := d2.GetTrajectory(ctx, tr.ID); err != nil || got.TemplateRef != tr.TemplateRef {
		t.Fatalf("get after reopen = %+v err=%v", got, err)
	}
	_ = d2.Close()

	// Corrupt the index: boot must quarantine it and rebuild from the sidecars.
	idxPath := filepath.Join(dir, dirTrajectories, trajectoryIndexFile)
	if err := os.WriteFile(idxPath, []byte("garbage"), 0o644); err != nil {
		t.Fatalf("corrupt index: %v", err)
	}
	d3, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen with corrupt index: %v", err)
	}
	defer d3.Close()
	if rows := d3.ListTrajectories(ctx, TrajectoryFilter{}); len(rows) != 1 || rows[0].ID != tr.ID {
		t.Fatalf("index rebuilt from sidecars = %+v", rows)
	}
	quarantined, _ := filepath.Glob(idxPath + ".corrupt-*")
	if len(quarantined) != 1 {
		t.Fatalf("corrupt index must be quarantined, glob = %v", quarantined)
	}
}

// TestTrajectoryDeleteAndRootDelete: explicit delete removes sidecar + row;
// deleting the root session drops the row too (the sidecar dies with the dir).
func TestTrajectoryDeleteAndRootDelete(t *testing.T) {
	ctx := context.Background()
	d, _, root := newTrajStore(t)
	tr, _ := d.CreateTrajectory(ctx, planDevTest(root))

	var ops []string
	d.SetTrajectoryHook(func(ev TrajectoryChangeEvent) { ops = append(ops, ev.Op+":"+ev.TrajectoryID) })

	if err := d.DeleteTrajectory(ctx, tr.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if d.trajectorySidecar(root).Exists() {
		t.Fatal("delete must remove the sidecar")
	}
	if _, err := d.GetTrajectory(ctx, tr.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: err=%v, want ErrNotFound", err)
	}
	if err := d.DeleteTrajectory(ctx, tr.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete: err=%v, want ErrNotFound", err)
	}
	// The root may get a new trajectory afterwards.
	tr2, err := d.CreateTrajectory(ctx, planDevTest(root))
	if err != nil {
		t.Fatalf("re-create after delete: %v", err)
	}
	if err := d.DeleteSession(ctx, root); err != nil {
		t.Fatalf("delete root session: %v", err)
	}
	if rows := d.ListTrajectories(ctx, TrajectoryFilter{}); len(rows) != 0 {
		t.Fatalf("deleting the root session must drop its trajectory row, got %+v", rows)
	}
	if _, err := d.GetTrajectory(ctx, tr2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after root delete: err=%v, want ErrNotFound", err)
	}
	want := []string{"delete:" + tr.ID, "create:" + tr2.ID, "delete:" + tr2.ID}
	if len(ops) != len(want) {
		t.Fatalf("hook ops = %v, want %v", ops, want)
	}
	for i := range want {
		if ops[i] != want[i] {
			t.Fatalf("hook ops = %v, want %v", ops, want)
		}
	}
}

// TestTrajectoryCorruptSidecar: a corrupt graph file is quarantined, reported
// once, and the trajectory then reads as gone (index row dropped).
func TestTrajectoryCorruptSidecar(t *testing.T) {
	ctx := context.Background()
	d, _, root := newTrajStore(t)
	tr, _ := d.CreateTrajectory(ctx, planDevTest(root))
	if err := os.WriteFile(d.TrajectoryPath(root), []byte("{"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := d.GetTrajectory(ctx, tr.ID); !IsSidecarCorrupt(err) {
		t.Fatalf("first read: err=%v, want SidecarCorruptError", err)
	}
	if _, err := d.GetTrajectory(ctx, tr.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second read: err=%v, want ErrNotFound", err)
	}
	if rows := d.ListTrajectories(ctx, TrajectoryFilter{}); len(rows) != 0 {
		t.Fatalf("index row must be dropped after quarantine, got %+v", rows)
	}
	// And the root can start over.
	if _, err := d.CreateTrajectory(ctx, planDevTest(root)); err != nil {
		t.Fatalf("re-create after quarantine: %v", err)
	}
}
