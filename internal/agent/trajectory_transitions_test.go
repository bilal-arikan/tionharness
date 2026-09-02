package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func trajSnapshot(id, status string, phases map[string]string) db.Trajectory {
	t := db.Trajectory{ID: id, RootSessionID: "SES1", TemplateRef: "plan-dev@3", Status: status}
	for pid, st := range phases {
		t.Nodes = append(t.Nodes, db.TrajectoryNode{ID: "p:" + pid, Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: st})
	}
	return t
}

// TestTrajectoryTransitionDiffer: create is a baseline; later snapshots yield
// enter / exit per phase and one end transition; delete forgets; an unknown
// trajectory's first update is a baseline too (boot case).
func TestTrajectoryTransitionDiffer(t *testing.T) {
	var d trajTransitionDiffer
	ev := func(op string, tr db.Trajectory) db.TrajectoryChangeEvent {
		return db.TrajectoryChangeEvent{TrajectoryID: tr.ID, RootSessionID: tr.RootSessionID, Op: op, Trajectory: tr}
	}
	if got := d.diff(ev(db.TrajectoryOpCreate, trajSnapshot("RTA1", db.TrajStatusPlanned, map[string]string{"plan": "pending", "code": "pending"}))); len(got) != 0 {
		t.Fatalf("create must be a baseline, got %+v", got)
	}
	got := d.diff(ev(db.TrajectoryOpUpdate, trajSnapshot("RTA1", db.TrajStatusRunning, map[string]string{"plan": "active", "code": "pending"})))
	if len(got) != 1 || got[0].Kind != TrajTransitionPhase || got[0].PhaseID != "plan" || got[0].Event != db.TrajEventEnter {
		t.Fatalf("plan enter = %+v", got)
	}
	if got[0].RecipeSlug() != "plan-dev" {
		t.Fatalf("recipe slug = %q", got[0].RecipeSlug())
	}
	got = d.diff(ev(db.TrajectoryOpUpdate, trajSnapshot("RTA1", db.TrajStatusRunning, map[string]string{"plan": "done", "code": "active"})))
	if len(got) != 2 {
		t.Fatalf("plan exit + code enter expected, got %+v", got)
	}
	var exit, enter bool
	for _, tr := range got {
		if tr.PhaseID == "plan" && tr.Event == db.TrajEventExit && tr.PhaseState == db.TrajStateDone {
			exit = true
		}
		if tr.PhaseID == "code" && tr.Event == db.TrajEventEnter {
			enter = true
		}
	}
	if !exit || !enter {
		t.Fatalf("transitions = %+v", got)
	}
	// Same snapshot again: nothing new.
	if got := d.diff(ev(db.TrajectoryOpUpdate, trajSnapshot("RTA1", db.TrajStatusRunning, map[string]string{"plan": "done", "code": "active"}))); len(got) != 0 {
		t.Fatalf("no change must yield nothing, got %+v", got)
	}
	got = d.diff(ev(db.TrajectoryOpUpdate, trajSnapshot("RTA1", db.TrajStatusDone, map[string]string{"plan": "done", "code": "done"})))
	if len(got) != 2 || got[1].Kind != TrajTransitionEnd || got[1].Status != db.TrajStatusDone {
		t.Fatalf("code exit + end expected, got %+v", got)
	}
	// Terminal → terminal never re-ends.
	if got := d.diff(ev(db.TrajectoryOpUpdate, trajSnapshot("RTA1", db.TrajStatusDone, map[string]string{"plan": "done", "code": "done"}))); len(got) != 0 {
		t.Fatalf("second terminal snapshot must be silent, got %+v", got)
	}
	d.diff(ev(db.TrajectoryOpDelete, db.Trajectory{ID: "RTA1"}))
	if _, ok := d.memo["RTA1"]; ok {
		t.Fatal("delete must forget the trajectory")
	}
	// Unknown id on update: baseline, no announcement.
	if got := d.diff(ev(db.TrajectoryOpUpdate, trajSnapshot("RTA9", db.TrajStatusDone, map[string]string{"plan": "done"}))); len(got) != 0 {
		t.Fatalf("first sight of an existing trajectory must be a baseline, got %+v", got)
	}
}
