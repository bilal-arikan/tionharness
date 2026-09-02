package view

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func sampleTrajectory() db.Trajectory {
	nodes := []db.TrajectoryNode{
		{ID: "p:plan", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, Label: "plan", State: db.TrajStateDone, StartMs: 1000, EndMs: 4000},
		{ID: "p:code", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, Label: "code", State: db.TrajStateActive, Gate: &db.TrajectoryGate{Kind: "verdict", Value: "pass"}},
		{ID: "p:review", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: db.TrajStatePending},
		{ID: "s:COORD", Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved, RefKind: "session", RefID: "COORD", PhaseID: "p:plan", State: db.TrajStateDone},
		{ID: "s:W1", Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved, RefKind: "session", RefID: "W1", PhaseID: "p:code", State: db.TrajStateActive, Label: "worker"},
		{ID: "a:AUT1", Kind: db.TrajNodeAutomation, Origin: db.TrajOriginDeclared, RefKind: "automation", RefID: "AUT1", State: db.TrajStateGhost},
		{ID: "r:RUN1", Kind: db.TrajNodeFlowRun, Origin: db.TrajOriginObserved, RefKind: "flowrun", RefID: "RUN1", State: db.TrajStateDone},
	}
	return db.Trajectory{
		ID: "RTA1", RootSessionID: "COORD", TemplateRef: "plan-dev-test@1", Revision: 7,
		Status: db.TrajStatusRunning, Nodes: nodes, UpdatedAt: 99,
		Edges: []db.TrajectoryEdge{{From: "p:plan", To: "p:code", Kind: db.TrajEdgeNext}},
	}
}

func TestProjectTrajectoryLevels(t *testing.T) {
	in := TrajectoryInput{Trajectory: sampleTrajectory(), Now: time.Unix(1_700_000_000, 0)}

	tiny, err := ProjectTrajectory(in, LevelTiny)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TRAJECTORY · RTA1", "root COORD", "plan-dev-test@1", "running", "rev 7"} {
		if !strings.Contains(tiny.Header, want) {
			t.Errorf("tiny header %q lacks %q", tiny.Header, want)
		}
	}
	if tiny.Body != "" || len(tiny.Handles) != 0 {
		t.Errorf("tiny must be header-only, got body %q handles %d", tiny.Body, len(tiny.Handles))
	}

	card, err := ProjectTrajectory(in, LevelCard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(card.Body, "✓ plan → ● code → ○ review") {
		t.Errorf("card phases line wrong:\n%s", card.Body)
	}
	if !strings.Contains(card.Body, "2 oturum · 1 akış koşusu · 1 otomasyon · 0 kapı · 1 hayalet") {
		t.Errorf("card counts wrong:\n%s", card.Body)
	}
	if !strings.Contains(card.Body, "4 ilan · 3 gözlem · 1 kenar") {
		t.Errorf("card origin line wrong:\n%s", card.Body)
	}
	if strings.Contains(card.Body, "kapı:verdict") {
		t.Errorf("card must not list per-phase detail:\n%s", card.Body)
	}
	if len(card.Handles) != 4 {
		t.Fatalf("card handles = %d, want 4 bound nodes", len(card.Handles))
	}
	if card.Handles[1].Ref != (Ref{Kind: KindSession, ID: "W1"}) || card.Handles[3].Ref != (Ref{Kind: KindFlowRun, ID: "RUN1"}) {
		t.Errorf("handle refs wrong: %+v", card.Handles)
	}

	full, err := ProjectTrajectory(in, LevelFull)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"● code [active] kapı:verdict=pass", "✓ plan [done]", "3s", "⌘ session W1 [active] ⊂ p:code", "⚡ automation AUT1 [ghost]"} {
		if !strings.Contains(full.Body, want) {
			t.Errorf("full body lacks %q:\n%s", want, full.Body)
		}
	}
}

func TestProjectTrajectoryElidesHandles(t *testing.T) {
	tr := sampleTrajectory()
	for i := 0; i < trajectoryTopN+3; i++ {
		id := "X" + string(rune('a'+i))
		tr.Nodes = append(tr.Nodes, db.TrajectoryNode{ID: "s:" + id, Kind: db.TrajNodeSession, RefKind: "session", RefID: id, State: db.TrajStateDone})
	}
	v, err := ProjectTrajectory(TrajectoryInput{Trajectory: tr}, LevelCard)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Handles) != trajectoryTopN || v.Elided != 7 || v.ElidedUnit != "düğüm" {
		t.Errorf("handles=%d elided=%d unit=%q", len(v.Handles), v.Elided, v.ElidedUnit)
	}
}

func TestProjectTrajectoryRejectsEmptyID(t *testing.T) {
	if _, err := ProjectTrajectory(TrajectoryInput{}, LevelCard); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestTrajectoryChildrenAndProjection(t *testing.T) {
	now := time.Now().Unix()
	p := NewProjector(&fakeStore{
		agents: []db.Agent{{ID: "AG1", Name: "builder"}},
		sessions: []db.Session{
			{ID: "COORD", AgentID: "AG1", UpdatedAt: now, CoordinatorMode: true},
			{ID: "W1", AgentID: "AG1", UpdatedAt: now, CoordinatorSessionID: "COORD"},
			{ID: "S3", AgentID: "AG1", UpdatedAt: now},
		},
		trajectories: []db.Trajectory{sampleTrajectory()},
	})
	ctx := context.Background()

	if !IsExpandable(Ref{Kind: KindTrajectory, ID: "RTA1"}) {
		t.Fatal("trajectory must be expandable")
	}

	// Root session: the trajectory handle leads, then the workers.
	kids, err := p.Children(ctx, Ref{Kind: KindSession, ID: "COORD"})
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) < 2 || kids[0].Ref != (Ref{Kind: KindTrajectory, ID: "RTA1"}) {
		t.Fatalf("session children = %+v", kids)
	}
	if !strings.Contains(kids[0].Label, "rota:RTA1") || !strings.Contains(kids[0].Label, "[running]") {
		t.Errorf("trajectory handle label = %q", kids[0].Label)
	}
	// A session without a trajectory is unchanged.
	kids, err = p.Children(ctx, Ref{Kind: KindSession, ID: "S3"})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range kids {
		if h.Ref.Kind == KindTrajectory {
			t.Fatalf("S3 must not expose a trajectory: %+v", kids)
		}
	}

	// Trajectory children are its bound entities, in node order.
	kids, err = p.Children(ctx, Ref{Kind: KindTrajectory, ID: "RTA1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Ref{{Kind: KindSession, ID: "COORD"}, {Kind: KindSession, ID: "W1"}, {Kind: KindAutomation, ID: "AUT1"}, {Kind: KindFlowRun, ID: "RUN1"}}
	if len(kids) != len(want) {
		t.Fatalf("trajectory children = %+v", kids)
	}
	for i, h := range kids {
		if h.Ref != want[i] {
			t.Errorf("child %d = %+v, want %+v", i, h.Ref, want[i])
		}
	}
	if _, err := p.Children(ctx, Ref{Kind: KindTrajectory, ID: "NOPE"}); err == nil {
		t.Fatal("missing trajectory must error")
	}

	// Project dispatches by kind.
	v, err := p.Project(ctx, Ref{Kind: KindTrajectory, ID: "RTA1"}, LevelCard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v.Header, "TRAJECTORY · RTA1") {
		t.Errorf("header = %q", v.Header)
	}
}
