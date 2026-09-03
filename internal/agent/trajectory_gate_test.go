package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/trajectory"
)

func gateTrajectory(t *testing.T, rt *Runtime, gate *db.TrajectoryGate) (db.Session, db.Trajectory) {
	t.Helper()
	ctx := context.Background()
	ag, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	root, err := rt.db.CreateSession(ctx, db.Session{AgentID: ag.ID, Title: "root", CoordinatorMode: true})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := rt.db.CreateTrajectory(ctx, db.Trajectory{
		RootSessionID: root.ID, TemplateRef: "plan-dev@3", Status: db.TrajStatusRunning,
		Nodes: []db.TrajectoryNode{
			{ID: "p:plan", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: db.TrajStateActive, Gate: gate},
			{ID: "p:code", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: db.TrajStatePending},
			{ID: "s:" + root.ID, Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved, RefKind: "session", RefID: root.ID, State: db.TrajStateActive},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return root, tr
}

// TestPhaseGateArtifactAndVerdict: an artifact gate passes once an artifact
// with the title exists in the tree, a verdict gate once the line is in the
// root transcript; until then moving to done is refused unless forced.
func TestPhaseGateArtifactAndVerdict(t *testing.T) {
	rt := lifecycleRuntime(t)
	ctx := context.Background()
	root, tr := gateTrajectory(t, rt, &db.TrajectoryGate{Kind: "artifact", Value: "plan"})
	if _, err := rt.SetTrajectoryPhase(ctx, tr.ID, "plan", db.TrajStateDone, "", 0, false); !errors.Is(err, ErrGateBlocked) {
		t.Fatalf("artifact gate must block without the artifact, got %v", err)
	}
	if _, err := rt.db.CreateArtifact(ctx, db.Artifact{SessionID: root.ID, Title: "Implementation plan", Kind: "markdown", Content: "…"}); err != nil {
		t.Fatal(err)
	}
	got, err := rt.SetTrajectoryPhase(ctx, tr.ID, "plan", db.TrajStateDone, "", 0, false)
	if err != nil || trajectory.NodePtr(&got, "p:plan").State != db.TrajStateDone {
		t.Fatalf("artifact gate must pass with the artifact: %v / %+v", err, trajectory.NodePtr(&got, "p:plan"))
	}

	rt2 := lifecycleRuntime(t)
	root2, tr2 := gateTrajectory(t, rt2, &db.TrajectoryGate{Kind: "verdict", Value: "VERDICT: PASS"})
	if _, err := rt2.SetTrajectoryPhase(ctx, tr2.ID, "plan", db.TrajStateDone, "", 0, false); !errors.Is(err, ErrGateBlocked) {
		t.Fatalf("verdict gate must block without the line, got %v", err)
	}
	// force overrides.
	if _, err := rt2.SetTrajectoryPhase(ctx, tr2.ID, "plan", db.TrajStateDone, "reviewed by hand", 0, true); err != nil {
		t.Fatalf("force must override: %v", err)
	}
	// A fresh verdict gate on the next phase passes once the transcript says so.
	_, _ = rt2.db.UpdateTrajectory(ctx, tr2.ID, 0, func(t *db.Trajectory) error {
		trajectory.NodePtr(t, "p:code").Gate = &db.TrajectoryGate{Kind: "verdict", Value: "verdict: pass"}
		return trajectory.SetPhaseState(t, "code", db.TrajStateActive, "", 1)
	})
	if _, err := rt2.db.AddMessage(ctx, db.Message{SessionID: root2.ID, Role: "user", Text: "validator says VERDICT: PASS (3 tests green)"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rt2.SetTrajectoryPhase(ctx, tr2.ID, "code", db.TrajStateDone, "", 0, false); err != nil {
		t.Fatalf("verdict gate must pass once the line is in the transcript: %v", err)
	}
	// Stale revision is refused by the CAS.
	if _, err := rt2.SetTrajectoryPhase(ctx, tr2.ID, "code", db.TrajStateSkipped, "", 1, false); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("stale expectedRev must conflict, got %v", err)
	}
}

// TestPhaseGateHuman: a human gate parks one durable ask on the root (a repeat
// request re-uses it), the trajectory waits with a gate node under the phase,
// and the answer resolves it — approval closes the phase, rejection keeps it
// active with the reason.
func TestPhaseGateHuman(t *testing.T) {
	rt := lifecycleRuntime(t)
	ctx := context.Background()
	root, tr := gateTrajectory(t, rt, &db.TrajectoryGate{Kind: "human", Value: "PM onayı"})
	_, err := rt.SetTrajectoryPhase(ctx, tr.ID, "plan", db.TrajStateDone, "", 0, false)
	if !errors.Is(err, ErrGatePending) {
		t.Fatalf("human gate must report pending, got %v", err)
	}
	// Second request: same ask, no duplicate.
	_, _ = rt.SetTrajectoryPhase(ctx, tr.ID, "plan", db.TrajStateDone, "", 0, false)
	asks, _ := rt.db.ListWaitingSessionAsks(ctx)
	var gate db.SessionAsk
	n := 0
	for _, a := range asks {
		if a.SessionID == root.ID {
			n++
			gate = a
		}
	}
	if n != 1 {
		t.Fatalf("exactly one gate ask expected on the root, got %d", n)
	}
	tid, phase, ok := GateAskRef(gate)
	if !ok || tid != tr.ID || phase != "plan" || !strings.Contains(gate.Payload, "Onayla") {
		t.Fatalf("gate ask = %+v (ok=%v tid=%s phase=%s)", gate, ok, tid, phase)
	}
	got, _ := rt.db.GetTrajectory(ctx, tr.ID)
	gn := trajectory.NodePtr(&got, "g:"+gate.ID)
	if got.Status != db.TrajStatusWaiting || gn == nil || gn.PhaseID != "p:plan" || gn.Label != "kapı: plan" {
		t.Fatalf("waiting graph = %s / %+v", got.Status, gn)
	}
	// Rejection keeps the phase active with the answer.
	claimed, err := rt.db.ClaimSessionAsk(ctx, gate.ID, "Reddet — plan eksik")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.ResolvePhaseGate(ctx, claimed, "Reddet — plan eksik"); err != nil {
		t.Fatal(err)
	}
	got, _ = rt.db.GetTrajectory(ctx, tr.ID)
	if p := trajectory.NodePtr(&got, "p:plan"); p.State != db.TrajStateActive || !strings.Contains(p.Reason, "reddedildi") || got.Status != db.TrajStatusRunning {
		t.Fatalf("after rejection = %+v / %s", p, got.Status)
	}
	if g := trajectory.NodePtr(&got, "g:"+gate.ID); g.State != db.TrajStateDone {
		t.Fatalf("gate node after answer = %+v", g)
	}
	// Approval closes the phase.
	_, err = rt.SetTrajectoryPhase(ctx, tr.ID, "plan", db.TrajStateDone, "", 0, false)
	if !errors.Is(err, ErrGatePending) {
		t.Fatalf("second gate must open, got %v", err)
	}
	asks, _ = rt.db.ListWaitingSessionAsks(ctx)
	for _, a := range asks {
		if a.SessionID == root.ID {
			claimed, _ = rt.db.ClaimSessionAsk(ctx, a.ID, "Onayla")
		}
	}
	if err := rt.ResolvePhaseGate(ctx, claimed, "Onayla"); err != nil {
		t.Fatal(err)
	}
	got, _ = rt.db.GetTrajectory(ctx, tr.ID)
	if p := trajectory.NodePtr(&got, "p:plan"); p.State != db.TrajStateDone || p.Reason != "kapı onaylandı" {
		t.Fatalf("after approval = %+v", p)
	}
	// The root transcript carries the decision.
	last, _, _ := rt.db.LastMessage(ctx, root.ID)
	if !strings.Contains(last.Text, "approved=true") {
		t.Fatalf("gate note missing: %q", last.Text)
	}
	if rt.trajectoryWorkPending() {
		t.Fatal("no queued work expected")
	}
}
