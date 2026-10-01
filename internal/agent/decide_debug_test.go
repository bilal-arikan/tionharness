package agent

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
)

func TestStallDecisionDebugPreservesBackgroundSession(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	hub := wireDecider(t, tun, stub, map[string]decider.Mode{authStallJudge: decider.ModeOn})
	stub.set(func(s *decisionStub) { s.noul[stallQuestionKey] = 0.94 })
	const sessionID = "SES-background-stall"
	const text = "Round 5 opened - 2 arms [running]"
	stalled, err := rt.judgeCoordinatorStalled(context.Background(), sessionID, db.Agent{ID: "AGT1"}, text)
	if err != nil || !stalled {
		t.Fatalf("stalled = %v, err = %v", stalled, err)
	}
	report := hub.Debug(decider.DebugFilter{Ref: sessionID})
	if report.Summary.Decisions != 1 || report.Summary.Applied != 1 {
		t.Fatalf("background decision was not correlated: %+v", report.Summary)
	}
	for _, event := range report.Events {
		if event.SessionID != sessionID {
			t.Errorf("stage %s session = %q", event.Stage, event.SessionID)
		}
		if event.Stage == "outcome" && (event.Ref != sessionID || event.Outcome != "stalled") {
			t.Errorf("outcome = %+v", event)
		}
	}
	// A repeated sweeper pass must reuse the memo, without another paid decision
	// or a duplicate debug trace for the same unchanged coordinator message.
	_, _ = rt.judgeCoordinatorStalled(context.Background(), sessionID, db.Agent{ID: "AGT1"}, text)
	if stub.calls() != 1 || hub.Debug(decider.DebugFilter{Ref: sessionID}).Summary.Decisions != 1 {
		t.Fatal("the cached stall verdict created another decision")
	}
}
