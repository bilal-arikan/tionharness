package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/trajectory"
)

func TestFlowJudgeBranchMapsArms(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	hub := wireDecider(t, tun, stub, nil) // flow-judge is on by default once enabled
	f := flowRunner{rt: rt}
	req := orchestration.JudgeRequest{Value: "Login page crashes on submit", Options: []string{"Bug report", "Feature request", "Question"}}

	stub.set(func(s *decisionStub) { s.choice[flowArmKey] = "arm1" })
	pick, conf, err := f.JudgeBranch(context.Background(), req)
	if err != nil || pick != 0 || conf != 0.9 {
		t.Fatalf("pick = %d conf = %v err = %v", pick, conf, err)
	}
	body := stub.bodies[len(stub.bodies)-1]
	if !strings.Contains(body, `"arm2":"Feature request"`) || !strings.Contains(body, "Which option best describes") {
		t.Errorf("request body = %s", body)
	}
	// An option key the model made up is "unsure", not a crash.
	stub.set(func(s *decisionStub) { s.choice[flowArmKey] = "arm9" })
	if pick, _, err := f.JudgeBranch(context.Background(), req); err != nil || pick != -1 {
		t.Errorf("unknown option: pick = %d, err = %v", pick, err)
	}
	if recs := hub.Recent(1); len(recs) != 1 || recs[0].Outcome != "unsure" {
		t.Errorf("ledger = %+v", recs)
	}
	// Disabled decider: an error the engine turns into the default arm.
	cfg := hub.Config()
	cfg.Enabled = false
	_, _ = hub.Update(cfg)
	if _, _, err := f.JudgeBranch(context.Background(), req); !errors.Is(err, decider.ErrDisabled) {
		t.Errorf("disabled decider err = %v", err)
	}
}

func TestFlowJudgeCondition(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	wireDecider(t, tun, stub, nil)
	f := flowRunner{rt: rt}
	stub.set(func(s *decisionStub) { s.noul[flowConditionKey] = 0.92 })
	holds, p, err := f.JudgeCondition(context.Background(), orchestration.JudgeRequest{Value: "LGTM, ship it", Condition: "The reviewer approved the change"})
	if err != nil || !holds || p != 0.92 {
		t.Fatalf("holds = %v p = %v err = %v", holds, p, err)
	}
	stub.set(func(s *decisionStub) { s.noul[flowConditionKey] = 0.3 })
	if holds, _, _ := f.JudgeCondition(context.Background(), orchestration.JudgeRequest{Value: "needs work", Condition: "approved"}); holds {
		t.Error("condition held below the threshold")
	}
}

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
