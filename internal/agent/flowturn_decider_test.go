package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/flow"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// okProvider answers every completion with "ok" (or a per-prompt override).
func okProvider(reply func(u string) string) *flowTestProvider {
	p := &flowTestProvider{}
	p.reply = func(call int, req providers.Request) (*providers.Response, error) {
		text := "ok"
		if reply != nil {
			text = reply(lastUser(req))
		}
		return &providers.Response{Text: text, StopReason: providers.StopEndTurn, Model: "flow-model", Usage: providers.Usage{InputTokens: 1, OutputTokens: 1}}, nil
	}
	return p
}

// waitUntil polls cond for up to 5s.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func flowTrace(t *testing.T, run db.FlowRun) []flow.Step {
	t.Helper()
	var trace []flow.Step
	if err := json.Unmarshal(run.Steps, &trace); err != nil {
		t.Fatalf("trace: %v", err)
	}
	return trace
}

// TestFlowCriteriaRouteUsesDecider: a criteria route asks the decider one
// yes/no question per criterion; a failed criterion takes the fail arm (and
// is named in the step detail), all passing takes the pass arm.
func TestFlowCriteriaRouteUsesDecider(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	hub := wireDecider(t, tun, stub, map[string]decider.Mode{authFlowCriteria: decider.ModeOn})
	installFlowTestProvider(t, rt, okProvider(func(u string) string {
		if strings.HasPrefix(u, "Fix:") {
			return "fixed"
		}
		return "draft"
	}))
	a := flowTestAgent(t, rt, "C")
	ctx := context.Background()
	f, _ := rt.db.EnsureAgentFlow(ctx, a.ID)
	ops := []flow.Op{
		{Op: "insert_between", From: "respond", To: "output", Node: &flow.Node{ID: "gate", Type: flow.NodeRoute, Mode: flow.ModeCriteria, Criteria: []string{"is polite", "is short"}, MaxVisits: 1}},
		{Op: "update_edge", ID: "e_gate_output", Edge: &flow.Edge{When: "pass"}},
		{Op: "add_node", Node: &flow.Node{ID: "fix", Type: flow.NodeLLM, Context: flow.ContextFresh, Tools: flow.ToolsNone, Prompt: "Fix: {{last}}"}},
		{Op: "add_edge", Edge: &flow.Edge{From: "gate", To: "fix", When: "fail"}},
		{Op: "add_edge", Edge: &flow.Edge{From: "fix", To: "gate"}},
		{Op: "add_edge", Edge: &flow.Edge{ID: "e_gate_default", From: "gate", To: "output"}},
	}
	if _, err := rt.ApplyFlowOps(ctx, f.ID, ops, db.FlowAuthor{Kind: db.FlowAuthorUser}, "criteria gate", ""); err != nil {
		t.Fatalf("apply ops: %v", err)
	}
	// One criterion fails → fail arm → fix → gate again (visit cap) → default.
	stub.set(func(s *decisionStub) { s.noul["c1"] = 0.9; s.noul["c2"] = 0.2 })
	resp, _, err := runTurn(t, rt, a, "hello")
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if resp.Text != "fixed" {
		t.Fatalf("reply = %q", resp.Text)
	}
	runs, _ := rt.db.ListFlowRuns(ctx, f.ID, 0)
	trace := flowTrace(t, runs[0])
	if trace[2].Edge != "fail" || !strings.Contains(trace[2].Detail, "failed: is short") {
		t.Fatalf("gate step = %+v", trace[2])
	}
	if trace[4].Edge != "* (visit cap)" {
		t.Fatalf("second gate visit must hit the cap: %+v", trace[4])
	}
	if body := stub.bodies[0]; !strings.Contains(body, `"c1"`) || !strings.Contains(body, `"c2"`) || !strings.Contains(body, "is polite") {
		t.Errorf("request body = %s", body)
	}
	recs := hub.Recent(1)
	if len(recs) != 1 || recs[0].Authority != authFlowCriteria || recs[0].Outcome != "fail" || recs[0].Ref != runs[0].ID {
		t.Errorf("ledger = %+v", recs)
	}
	// Everything passes → straight to the output.
	stub.set(func(s *decisionStub) { s.noul["c2"] = 0.95 })
	resp, _, err = runTurn(t, rt, a, "hello")
	if err != nil || resp.Text != "draft" {
		t.Fatalf("pass turn: %v %q", err, resp.Text)
	}
	runs, _ = rt.db.ListFlowRuns(ctx, f.ID, 0)
	if trace := flowTrace(t, runs[0]); trace[2].Edge != "pass" || !strings.Contains(trace[2].Detail, "2/2") {
		t.Fatalf("pass step = %+v", trace[2])
	}
	waitBackground(rt)
}

// TestFlowRunGradingFiresFlowAutomation: with grading on, a finished run gets
// a grade; a flow-kind automation narrowed to that grade fires once with the
// run rendered into its prompt, and the session it spawns does not re-fire it.
func TestFlowRunGradingFiresFlowAutomation(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t) // score answers are level 2 → grade 3
	wireDecider(t, tun, stub, map[string]decider.Mode{authFlowGrade: decider.ModeOn})
	installFlowTestProvider(t, rt, okProvider(nil))
	a := flowTestAgent(t, rt, "G")
	reviewer := flowTestAgent(t, rt, "R")
	ctx := context.Background()
	f, _ := rt.db.EnsureAgentFlow(ctx, a.ID)
	engine := NewAutomationEngine(rt.db, rt, rt.logger)
	rt.AddFlowRunHook(engine.OnFlowRunFinished)
	rule, err := rt.db.CreateAutomation(ctx, db.Automation{
		Name: "review weak replies", TriggerKind: db.TriggerFlow, FlowMaxGrade: 3,
		TargetAgentID: reviewer.ID, PromptTemplate: "Review run {{runId}} of {{agent}} (grade {{grade}}, {{status}}): {{result}}",
		Enabled: true, MaxIterations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runTurn(t, rt, a, "hello"); err != nil {
		t.Fatal(err)
	}
	waitBackground(rt) // grading runs on the detached post-run tail
	runs, _ := rt.db.ListFlowRuns(ctx, f.ID, 0)
	if len(runs) != 1 || runs[0].Grade != 3 || runs[0].GradeConfidence < 0.5 {
		t.Fatalf("run not graded: %+v", runs[0])
	}
	if cur, _ := rt.db.GetFlow(ctx, f.ID); cur.Stats.Graded != 1 || cur.Stats.GradeSum != 3 {
		t.Fatalf("stats = %+v", cur.Stats)
	}
	waitUntil(t, "the flow rule to fire", func() bool {
		got, _ := rt.db.GetAutomation(ctx, rule.ID)
		return got.IterationCount == 1
	})
	drainSpawns(t, rt)
	sessions, _ := rt.db.ListSessions(ctx, reviewer.ID)
	if len(sessions) != 1 {
		t.Fatalf("reviewer sessions = %d", len(sessions))
	}
	msgs, _ := rt.db.ListMessages(ctx, sessions[0].ID)
	if len(msgs) == 0 || !strings.Contains(msgs[0].Text, "Review run "+runs[0].ID+" of G (grade 3, success): ok") {
		t.Fatalf("spawned prompt = %+v", msgs)
	}
	if o := sessions[0].Origin; o == nil || o.Kind != db.OriginAutomation || o.EntityID != rule.ID {
		t.Fatalf("origin = %+v", sessions[0].Origin)
	}
	// The reviewer's own run (spawned by the rule) matched the rule too — any
	// agent, grade 3 — but must not re-fire it.
	waitBackground(rt)
	time.Sleep(150 * time.Millisecond)
	drainSpawns(t, rt)
	if got, _ := rt.db.GetAutomation(ctx, rule.ID); got.IterationCount != 1 {
		t.Fatalf("rule re-fired from its own session: %+v", got)
	}
	// A rule narrowed to another agent never matches.
	if flowRuleMatches(db.Automation{FlowAgentID: "AGT-x"}, runs[0]) || !flowRuleMatches(db.Automation{FlowStatus: db.FlowSuccess}, runs[0]) ||
		flowRuleMatches(db.Automation{FlowMaxGrade: 2}, runs[0]) || flowRuleMatches(db.Automation{FlowMaxGrade: 3}, db.FlowRun{Status: db.FlowSuccess}) {
		t.Error("flowRuleMatches filters wrong")
	}
}

// TestFlowTriggerNodeFiresAutomation: a trigger node fires an automation with
// its rendered payload, passes the reply through untouched and records what
// happened on the step; a disabled automation is skipped, not an error.
func TestFlowTriggerNodeFiresAutomation(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	installFlowTestProvider(t, rt, okProvider(nil))
	a := flowTestAgent(t, rt, "T")
	handler := flowTestAgent(t, rt, "H")
	ctx := context.Background()
	f, _ := rt.db.EnsureAgentFlow(ctx, a.ID)
	rule, err := rt.db.CreateAutomation(ctx, db.Automation{
		Name: "handoff", TriggerTag: "handoff", TargetAgentID: handler.ID,
		PromptTemplate: "Handle: {{result}}", SpawnTags: []string{}, Enabled: true, MaxIterations: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	ops := []flow.Op{{Op: "insert_between", From: "respond", To: "output", Node: &flow.Node{ID: "notify", Type: flow.NodeTrigger, AutomationID: rule.ID, Template: "Reply: {{last}}"}}}
	if _, err := rt.ApplyFlowOps(ctx, f.ID, ops, db.FlowAuthor{Kind: db.FlowAuthorUser}, "trigger", ""); err != nil {
		t.Fatalf("apply ops: %v", err)
	}
	resp, _, err := runTurn(t, rt, a, "hello")
	if err != nil || resp.Text != "ok" {
		t.Fatalf("turn: %v %q", err, resp.Text)
	}
	runs, _ := rt.db.ListFlowRuns(ctx, f.ID, 0)
	trace := flowTrace(t, runs[0])
	if trace[2].Type != flow.NodeTrigger || !strings.HasPrefix(trace[2].Detail, "triggered handoff") || trace[2].Input != "Reply: ok" {
		t.Fatalf("trigger step = %+v", trace[2])
	}
	drainSpawns(t, rt)
	sessions, _ := rt.db.ListSessions(ctx, handler.ID)
	if len(sessions) != 1 {
		t.Fatalf("handler sessions = %d", len(sessions))
	}
	if msgs, _ := rt.db.ListMessages(ctx, sessions[0].ID); len(msgs) == 0 || msgs[0].Text != "Handle: Reply: ok" {
		t.Fatalf("handoff prompt = %+v", msgs)
	}
	if got, _ := rt.db.GetAutomation(ctx, rule.ID); got.IterationCount != 1 {
		t.Fatalf("rule bookkeeping = %+v", got)
	}
	// Disabled automation: the node reports the skip and the turn still answers.
	if err := rt.db.SetAutomationEnabled(ctx, rule.ID, false); err != nil {
		t.Fatal(err)
	}
	resp, _, err = runTurn(t, rt, a, "again")
	if err != nil || resp.Text != "ok" {
		t.Fatalf("turn 2: %v %q", err, resp.Text)
	}
	runs, _ = rt.db.ListFlowRuns(ctx, f.ID, 0)
	if trace := flowTrace(t, runs[0]); runs[0].Status != db.FlowSuccess || !strings.Contains(trace[2].Detail, "disabled") {
		t.Fatalf("disabled rule step = %+v (%s)", trace[2], runs[0].Status)
	}
	waitBackground(rt)
	drainSpawns(t, rt)
}

// TestFlowProposalGateHoldsAutoApply: with the gate on, a "no" keeps a
// confident proposal pending under the auto policy; a "yes" lets it apply.
func TestFlowProposalGateHoldsAutoApply(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	hub := wireDecider(t, tun, stub, map[string]decider.Mode{authFlowProposalGate: decider.ModeOn})
	proposal := `{"noChange":false,"reason":"add a critic","expected":"fewer errors","confidence":0.9,"ops":[{"op":"insert_between","from":"respond","to":"output","node":{"id":"critic","type":"llm","context":"fresh","tools":"none","prompt":"Review: {{node.respond}}"}}]}`
	installFlowTestProvider(t, rt, okProvider(func(u string) string {
		if strings.Contains(u, "# Flow") {
			return proposal
		}
		return "ok"
	}))
	a := flowTestAgent(t, rt, "P")
	ctx := context.Background()
	f, _ := rt.db.EnsureAgentFlow(ctx, a.ID)
	if _, _, err := runTurn(t, rt, a, "hello"); err != nil {
		t.Fatal(err)
	}
	pol := db.DefaultFlowPolicy()
	pol.Mode = db.FlowPolicyAuto
	if _, err := rt.db.UpdateFlowMeta(ctx, f.ID, nil, nil, &pol); err != nil {
		t.Fatal(err)
	}
	stub.set(func(s *decisionStub) { s.noul[flowProposalGateKey] = 0.2 })
	res, err := rt.OptimizeFlow(ctx, f.ID, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied || res.Held == "" || res.Proposal == nil || res.Proposal.Status != db.ProposalPending {
		t.Fatalf("held result = %+v", res)
	}
	if cur, _ := rt.db.GetFlow(ctx, f.ID); cur.Version != 1 {
		t.Fatalf("a held proposal must not change the head: v%d", cur.Version)
	}
	if recs := hub.Recent(1); len(recs) != 1 || recs[0].Outcome != "hold" || recs[0].Ref != res.Proposal.ID {
		t.Errorf("ledger = %+v", recs)
	}
	if body := stub.bodies[len(stub.bodies)-1]; !strings.Contains(body, "add a critic") || !strings.Contains(body, "+critic") {
		t.Errorf("gate state = %s", body)
	}
	stub.set(func(s *decisionStub) { s.noul[flowProposalGateKey] = 0.95 })
	res, err = rt.OptimizeFlow(ctx, f.ID, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || res.Held != "" {
		t.Fatalf("pass result = %+v", res)
	}
	if cur, _ := rt.db.GetFlow(ctx, f.ID); cur.Version != 2 {
		t.Fatalf("auto apply must commit: v%d", cur.Version)
	}
	waitBackground(rt)
}

// TestFlowObserverSkipsHealthyWindow: when every run of the window succeeded
// with a high grade, the observer pass is skipped and the marker still moves.
func TestFlowObserverSkipsHealthyWindow(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	p := okProvider(nil)
	installFlowTestProvider(t, rt, p)
	a := flowTestAgent(t, rt, "S")
	ctx := context.Background()
	f, _ := rt.db.EnsureAgentFlow(ctx, a.ID)
	pol := db.DefaultFlowPolicy()
	pol.EveryRuns = 2
	if _, err := rt.db.UpdateFlowMeta(ctx, f.ID, nil, nil, &pol); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, _, err := runTurn(t, rt, a, "hello"); err != nil {
			t.Fatal(err)
		}
		waitBackground(rt)
	}
	// Ungraded runs: the observer looked after the second turn (2 ≥ everyRuns)
	// and moved the marker. Grade the runs well, rewind the marker and ask again.
	runs, _ := rt.db.ListFlowRuns(ctx, f.ID, 0)
	for _, run := range runs {
		if _, err := rt.db.SetFlowRunGrade(ctx, run.ID, 5, 0.9); err != nil {
			t.Fatal(err)
		}
	}
	runs, _ = rt.db.ListFlowRuns(ctx, f.ID, 0)
	before := len(p.requests)
	rt.db.RewindFlowOptimizeMarkerForTest(f.ID)
	rt.maybeObserveFlow(ctx, f.ID)
	waitBackground(rt)
	if len(p.requests) != before {
		t.Fatalf("observer ran on a healthy window (%d new calls)", len(p.requests)-before)
	}
	if !flowRunsHealthy(runs, 2) || flowRunsHealthy(runs, 3) || flowRunsHealthy([]db.FlowRun{{Status: db.FlowSuccess, Grade: 5, Feedback: -1}}, 1) {
		t.Error("flowRunsHealthy rule wrong")
	}
}
