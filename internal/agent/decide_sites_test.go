package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/trajectory"
)

func TestPhaseGateJudge(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	hub := wireDecider(t, tun, stub, nil)
	ctx := context.Background()
	root, tr := gateTrajectory(t, rt, &db.TrajectoryGate{Kind: "judge", Value: "The validator approved the implementation"})

	// Empty transcript: blocked without asking.
	if _, err := rt.SetTrajectoryPhase(ctx, tr.ID, "plan", db.TrajStateDone, "", 0, false); !errors.Is(err, ErrGateBlocked) {
		t.Fatalf("empty transcript must block, got %v", err)
	}
	if _, err := rt.db.AddMessage(ctx, db.Message{SessionID: root.ID, Role: "assistant", Text: "Validator report:\n```\nVERDICT: PASS — all 12 tests green\n```"}); err != nil {
		t.Fatal(err)
	}
	stub.set(func(s *decisionStub) { s.noul[phaseGateKey] = 0.4 })
	_, err := rt.SetTrajectoryPhase(ctx, tr.ID, "plan", db.TrajStateDone, "", 0, false)
	if !errors.Is(err, ErrGateBlocked) || !strings.Contains(err.Error(), "condition not met") {
		t.Fatalf("low probability must block with a reason, got %v", err)
	}
	stub.set(func(s *decisionStub) { s.noul[phaseGateKey] = 0.93 })
	got, err := rt.SetTrajectoryPhase(ctx, tr.ID, "plan", db.TrajStateDone, "", 0, false)
	if err != nil || trajectory.NodePtr(&got, "p:plan").State != db.TrajStateDone {
		t.Fatalf("a met condition must open the gate: %v", err)
	}
	if !strings.Contains(stub.bodies[len(stub.bodies)-1], "VERDICT: PASS") {
		t.Error("the transcript was not sent as the state")
	}
	recs := hub.Recent(2)
	if len(recs) != 2 || recs[0].Outcome != "pass" || recs[1].Outcome != "blocked" || recs[0].Ref != tr.ID {
		t.Errorf("ledger = %+v", recs)
	}
}

func TestPhaseGateJudgeFailsClosed(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	hub := wireDecider(t, tun, stub, nil)
	cfg := hub.Config()
	cfg.Enabled = false
	_, _ = hub.Update(cfg)
	ctx := context.Background()
	root, tr := gateTrajectory(t, rt, &db.TrajectoryGate{Kind: "judge", Value: "done"})
	_, _ = rt.db.AddMessage(ctx, db.Message{SessionID: root.ID, Role: "assistant", Text: "all done"})
	_, err := rt.SetTrajectoryPhase(ctx, tr.ID, "plan", db.TrajStateDone, "", 0, false)
	if !errors.Is(err, ErrGateBlocked) || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("a judge gate with the decider off must stay blocked, got %v", err)
	}
}
