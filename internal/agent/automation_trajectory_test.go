package agent

import (
	"context"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/trajectory"
)

func TestTrajectoryRuleMatches(t *testing.T) {
	tr := TrajectoryTransition{
		Trajectory: db.Trajectory{ID: "RTA1", TemplateRef: "plan-dev@3"},
		Kind:       TrajTransitionPhase, PhaseID: "code", PhaseState: db.TrajStateDone, Event: db.TrajEventExit,
	}
	cases := []struct {
		name string
		a    db.Automation
		want bool
	}{
		{"any phase exit", db.Automation{TriggerKind: db.TriggerPhase}, true},
		{"phase filter matches", db.Automation{TriggerKind: db.TriggerPhase, TrajPhase: "code"}, true},
		{"phase filter case-insensitive", db.Automation{TriggerKind: db.TriggerPhase, TrajPhase: "Code"}, true},
		{"other phase", db.Automation{TriggerKind: db.TriggerPhase, TrajPhase: "plan"}, false},
		{"enter rule on exit", db.Automation{TriggerKind: db.TriggerPhase, TrajEvent: db.TrajEventEnter}, false},
		{"recipe filter matches slug without version", db.Automation{TriggerKind: db.TriggerPhase, TrajRecipe: "plan-dev"}, true},
		{"recipe filter mismatch", db.Automation{TriggerKind: db.TriggerPhase, TrajRecipe: "other"}, false},
		{"end rule on a phase transition", db.Automation{TriggerKind: db.TriggerTrajectoryEnd}, false},
		{"tag rule never", db.Automation{TriggerKind: db.TriggerTag}, false},
	}
	for _, c := range cases {
		if got := trajectoryRuleMatches(c.a, tr); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	end := TrajectoryTransition{Trajectory: db.Trajectory{ID: "RTA1", Status: db.TrajStatusFailed}, Kind: TrajTransitionEnd, Status: db.TrajStatusFailed}
	if !trajectoryRuleMatches(db.Automation{TriggerKind: db.TriggerTrajectoryEnd}, end) {
		t.Fatal("any-status end rule must match")
	}
	if trajectoryRuleMatches(db.Automation{TriggerKind: db.TriggerTrajectoryEnd, TrajStatus: db.TrajStatusDone}, end) {
		t.Fatal("status filter must apply")
	}
}

func TestTrajectoryVarsSubstitution(t *testing.T) {
	e := &AutomationEngine{}
	tr := TrajectoryTransition{
		Trajectory: db.Trajectory{ID: "RTA1", RootSessionID: "SES1", TemplateRef: "plan-dev@3", Status: db.TrajStatusRunning,
			Nodes: []db.TrajectoryNode{{ID: "p:plan", Kind: db.TrajNodePhase, State: db.TrajStateDone}}},
		Kind: TrajTransitionPhase, PhaseID: "plan", PhaseState: db.TrajStateDone, Event: db.TrajEventExit,
	}
	got := renderAutomationPrompt("{{trajectoryId}} {{rootSessionId}} {{recipe}} {{phase}}/{{phaseState}}/{{event}} {{status}} [{{phases}}]", e.trajectoryVars(db.Automation{}, tr))
	want := "RTA1 SES1 plan-dev plan/done/exit running [plan ✓]"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// waitNode polls the trajectory until the node reaches a non-ghost state.
func waitNode(t *testing.T, rt *Runtime, root, nodeID string) db.TrajectoryNode {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		tr, err := rt.db.GetTrajectoryByRoot(context.Background(), root)
		if err == nil {
			if n := trajectory.NodePtr(&tr, nodeID); n != nil && n.State != db.TrajStateGhost {
				return *n
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("node %s never left ghost on %s (err=%v)", nodeID, root, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestTrajectoryTransitionsFireAutomationsOntoGraph: a phase exit fires the
// recipe watcher bound to that phase and the explicit phase rule (both fail at
// launch — no agent — so their nodes read failed with the reason and the ledger
// holds the attempt); a trajectory end resolves a watcher that names no
// automation as skipped/not_found and a disabled one as skipped/disabled.
func TestTrajectoryTransitionsFireAutomationsOntoGraph(t *testing.T) {
	e := backstopEngine(t)
	rt := e.rt
	ctx := context.Background()
	rt.db.SetTrajectoryHook(rt.OnTrajectoryChange)
	rt.SetTrajectoryTransitionHook(e.OnTrajectoryTransition)

	ag, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	root, _ := rt.db.CreateSession(ctx, db.Session{AgentID: ag.ID, Title: "root", CoordinatorMode: true})
	docs := seedAutomation(t, e, db.Automation{Name: "docs", TriggerKind: db.TriggerTag, TargetAgentID: "missing-agent", MaxIterations: 5})
	rule := seedAutomation(t, e, db.Automation{Name: "on-code", TriggerKind: db.TriggerPhase, TrajPhase: "code", TargetAgentID: "missing-agent", MaxIterations: 5})
	off := seedAutomation(t, e, db.Automation{Name: "wrap-up", TriggerKind: db.TriggerTag, TargetAgentID: "missing-agent", MaxIterations: 5})
	if err := rt.db.SetAutomationEnabled(ctx, off.ID, false); err != nil {
		t.Fatal(err)
	}

	tr, err := rt.db.CreateTrajectory(ctx, db.Trajectory{
		RootSessionID: root.ID, TemplateRef: "plan-dev@3",
		Nodes: []db.TrajectoryNode{
			{ID: "p:plan", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: db.TrajStateActive},
			{ID: "p:code", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: db.TrajStatePending},
			{ID: "s:" + root.ID, Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved, RefKind: "session", RefID: root.ID, State: db.TrajStateActive},
			{ID: "a:docs@code", Kind: db.TrajNodeAutomation, Origin: db.TrajOriginDeclared, RefKind: "automation", RefID: "docs", PhaseID: "p:code", Lane: 1, State: db.TrajStateGhost},
			{ID: "a:nobody", Kind: db.TrajNodeAutomation, Origin: db.TrajOriginDeclared, RefKind: "automation", RefID: "nobody", Lane: 2, State: db.TrajStateGhost},
			{ID: "a:wrap-up", Kind: db.TrajNodeAutomation, Origin: db.TrajOriginDeclared, RefKind: "automation", RefID: "Wrap-Up", Lane: 3, State: db.TrajStateGhost},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// plan → done, code → active: exit(plan) + enter(code). Nothing watches plan's
	// exit or code's entry, so the graph must stay quiet.
	_, err = rt.db.UpdateTrajectory(ctx, tr.ID, 0, func(t *db.Trajectory) error {
		_ = trajectory.SetPhaseState(t, "code", db.TrajStateActive, "", 1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// code → done: exit(code) fires the docs watcher and the explicit rule.
	if _, err := rt.db.UpdateTrajectory(ctx, tr.ID, 0, func(t *db.Trajectory) error {
		return trajectory.SetPhaseState(t, "code", db.TrajStateDone, "", 2)
	}); err != nil {
		t.Fatal(err)
	}
	n := waitNode(t, rt, root.ID, "a:docs@code")
	if n.State != db.TrajStateFailed || n.RefID != docs.ID || n.Reason == "" || n.Label != "docs" {
		t.Fatalf("docs watcher node = %+v", n)
	}
	rn := waitNode(t, rt, root.ID, "a:"+rule.ID+"@code")
	if rn.State != db.TrajStateFailed || rn.PhaseID != "p:code" || rn.Origin != db.TrajOriginObserved {
		t.Fatalf("explicit rule node = %+v", rn)
	}
	got, _ := rt.db.GetAutomation(ctx, docs.ID)
	if got.IterationCount != 1 || got.LastError == "" {
		t.Fatalf("docs automation after fire attempt = iter %d err %q", got.IterationCount, got.LastError)
	}
	ledger, _ := rt.db.ListAutomationFires(ctx, rule.ID, 10)
	if len(ledger) != 1 || ledger[0].Outcome != db.AutomationFireFailed || ledger[0].TriggerKind != db.TriggerPhase {
		t.Fatalf("rule ledger = %+v", ledger)
	}

	// Trajectory ends: the unknown watcher is skipped/not_found, the disabled
	// one skipped/disabled (with a ledger entry), nothing else fires again.
	if _, err := rt.db.UpdateTrajectory(ctx, tr.ID, 0, func(t *db.Trajectory) error {
		t.Status = db.TrajStatusDone
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	nb := waitNode(t, rt, root.ID, "a:nobody")
	if nb.State != db.TrajStateSkipped || nb.Reason != AutomationSkipNotFound {
		t.Fatalf("unknown watcher node = %+v", nb)
	}
	wu := waitNode(t, rt, root.ID, "a:wrap-up")
	if wu.State != db.TrajStateSkipped || wu.Reason != db.AutomationSkipDisabled || wu.RefID != off.ID {
		t.Fatalf("disabled watcher node = %+v", wu)
	}
	offLedger, _ := rt.db.ListAutomationFires(ctx, off.ID, 10)
	if len(offLedger) != 1 || offLedger[0].Outcome != db.AutomationFireSkipped || offLedger[0].Reason != db.AutomationSkipDisabled {
		t.Fatalf("disabled watcher ledger = %+v", offLedger)
	}
	got, _ = rt.db.GetAutomation(ctx, docs.ID)
	if got.IterationCount != 1 {
		t.Fatalf("docs must not fire again at trajectory end, iter = %d", got.IterationCount)
	}
	for rt.trajectoryWorkPending() {
		time.Sleep(5 * time.Millisecond)
	}
}
