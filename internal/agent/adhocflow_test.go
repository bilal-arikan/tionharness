package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// adhocThreeStepSpec is the canonical plan: fan out a scan, branch on it, fan out
// a fix round only when the scan found something.
func adhocThreeStepSpec() tools.AdhocFlowSpec {
	return tools.AdhocFlowSpec{Steps: []tools.AdhocFlowStep{
		{ID: "scan", Type: tools.AdhocStepParallel, Next: "triage", Tasks: []tools.RunAgentTask{
			{Target: "explore", Task: "scan a"},
			{Target: "reviewer", Task: "scan b"},
		}},
		{ID: "triage", Type: tools.AdhocStepBranch, On: "scan", MatchMode: "contains", Branches: []tools.AdhocFlowBranch{
			{Value: "FAIL", Next: "fix"},
			{Value: "", Next: ""},
		}},
		{ID: "fix", Type: tools.AdhocStepParallel, Tasks: []tools.RunAgentTask{
			{Target: "coder", Task: "fix {{node.scan}}"},
		}},
	}}
}

// adhocStub is a fanOut stand-in: it records every round's spec and answers
// through reply, so the graph mechanics can be driven without a provider.
type adhocStub struct {
	mu    sync.Mutex
	specs []tools.RunAgentSpec
	reply func(ctx context.Context, round int, spec tools.RunAgentSpec) (tools.RunAgentResult, error)
}

func (s *adhocStub) fanOut(ctx context.Context, spec tools.RunAgentSpec) (tools.RunAgentResult, error) {
	s.mu.Lock()
	s.specs = append(s.specs, spec)
	round := len(s.specs)
	s.mu.Unlock()
	return s.reply(ctx, round, spec)
}

func (s *adhocStub) rounds() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.specs)
}

// answers builds an all-success fan-out result, one reply per leg.
func answers(spec tools.RunAgentSpec, reply string) tools.RunAgentResult {
	out := make([]tools.FanOutOutcome, len(spec.Tasks))
	for i, t := range spec.Tasks {
		out[i] = tools.FanOutOutcome{Index: i, Target: t.Target, AgentName: "subagent:" + t.Target, Reply: reply}
	}
	return tools.RunAgentResult{FanOut: out, Strategy: spec.Strategy}
}

// adhocFixture is a runtime plus a calling agent and a session-bearing context.
func adhocFixture(t *testing.T) (*Runtime, db.Agent, context.Context) {
	t.Helper()
	rt, _ := newTestRuntime(t, t.TempDir())
	caller, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Caller", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return rt, caller, WithSessionID(context.Background(), "SES-adhoc")
}

func stepStatuses(res tools.AdhocFlowResult) map[string]string {
	out := map[string]string{}
	for _, s := range res.Steps {
		out[s.ID] = s.Status
	}
	return out
}

// TestCompileAdhocFlowProducesValidGraph: a 3-step plan compiles to a graph that
// passes Validate, with each parallel step as ONE fan-out agent node.
func TestCompileAdhocFlowProducesValidGraph(t *testing.T) {
	g, err := compileAdhocFlow(adhocThreeStepSpec())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("compiled graph must validate: %v", err)
	}
	scan, ok := g.NodeByID("scan")
	if !ok || !scan.IsFanOut() || len(scan.Legs) != 2 || scan.Next != "triage" {
		t.Fatalf("scan should be one fan-out node with two legs, got %+v", scan)
	}
	triage, _ := g.NodeByID("triage")
	if triage.Type != orchestration.NodeTransform || triage.Template != "{{node.scan}}" || triage.Next != adhocRouteNodeID("triage") {
		t.Fatalf("triage should load the scan output into {{last}}, got %+v", triage)
	}
	route, _ := g.NodeByID(adhocRouteNodeID("triage"))
	if route.Type != orchestration.NodeBranch || route.MatchMode != "contains" || len(route.Branches) != 2 {
		t.Fatalf("triage route should be a contains branch, got %+v", route)
	}
}

// TestAdhocFlowRunsRoundsAndHidesItsFlow: the happy path — the branch routes to
// the fix round, the run is a real FlowRun on a hidden flow row.
func TestAdhocFlowRunsRoundsAndHidesItsFlow(t *testing.T) {
	rt, caller, ctx := adhocFixture(t)
	stub := &adhocStub{reply: func(_ context.Context, round int, spec tools.RunAgentSpec) (tools.RunAgentResult, error) {
		if round == 1 {
			return answers(spec, "verdict FAIL: race in store"), nil
		}
		return answers(spec, "fixed"), nil
	}}
	res, err := rt.runAdhocFlow(ctx, caller, false, adhocThreeStepSpec(), newAdhocRun(0, stub.fanOut))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Status != tools.AdhocStatusDone || res.Final != "fixed" {
		t.Fatalf("want done/fixed, got %+v", res)
	}
	got := stepStatuses(res)
	if got["scan"] != "done" || got["triage"] != "done" || got["fix"] != "done" {
		t.Fatalf("every step should have run, got %v", got)
	}
	if !strings.Contains(res.Steps[0].Output, "subagent:explore") || !strings.Contains(res.Steps[0].Output, "subagent:reviewer") {
		t.Fatalf("a parallel step's output must list each leg, got %q", res.Steps[0].Output)
	}
	if res.Steps[1].Output != "→ FAIL" {
		t.Fatalf("branch output should name the taken arm, got %q", res.Steps[1].Output)
	}
	// Round two read round one's answers through {{node.scan}}.
	if fixTask := stub.specs[1].Tasks[0].Task; !strings.Contains(fixTask, "race in store") {
		t.Fatalf("round-two legs should see round-one output, got %q", fixTask)
	}

	run, err := rt.db.GetFlowRun(context.Background(), res.RunID)
	if err != nil || run.Status != db.FlowSuccess {
		t.Fatalf("run_id must be a real, finished FlowRun: %+v %v", run, err)
	}
	flow, err := rt.db.GetFlow(context.Background(), run.FlowID)
	if err != nil || !flow.Ephemeral || flow.CreatedBy != caller.ID {
		t.Fatalf("the run must hang off an ephemeral flow row: %+v %v", flow, err)
	}
	list, _ := rt.db.ListFlows(context.Background())
	for _, f := range list {
		if f.ID == flow.ID {
			t.Fatal("the ephemeral flow leaked into the flow catalog")
		}
	}
	drainSpawns(t, rt)
}

// TestAdhocFlowRoundLegsCannotFanOut: every round hands runAgentFanOut plain
// legs, and legSpec turns each into a single-task spec — so a round-N leg can
// never fan out itself; repeat fan-out happens only at the graph level.
func TestAdhocFlowRoundLegsCannotFanOut(t *testing.T) {
	rt, caller, ctx := adhocFixture(t)
	stub := &adhocStub{reply: func(_ context.Context, _ int, spec tools.RunAgentSpec) (tools.RunAgentResult, error) {
		return answers(spec, "FAIL"), nil
	}}
	if _, err := rt.runAdhocFlow(ctx, caller, false, adhocThreeStepSpec(), newAdhocRun(0, stub.fanOut)); err != nil {
		t.Fatalf("run: %v", err)
	}
	if stub.rounds() != 2 {
		t.Fatalf("want two rounds, got %d", stub.rounds())
	}
	for r, spec := range stub.specs {
		if spec.Strategy != tools.StrategyAll || spec.RetryOf != "" {
			t.Fatalf("round %d: unexpected fan-out axes %+v", r+1, spec)
		}
		for _, leg := range spec.Tasks {
			single := legSpec(spec, leg)
			if len(single.Tasks) != 0 || single.Strategy != "" || single.MaxConcurrency != 0 {
				t.Fatalf("round %d: a leg must reach runAgent as a single task, got %+v", r+1, single)
			}
		}
	}
	drainSpawns(t, rt)
}

// TestAdhocFlowAllLegsFailedStopsTheRun: a round whose legs all fail is a node
// error — the next round must not start, and the untouched steps read "skipped",
// which is distinct from the "failed" round.
func TestAdhocFlowAllLegsFailedStopsTheRun(t *testing.T) {
	rt, caller, ctx := adhocFixture(t)
	stub := &adhocStub{reply: func(context.Context, int, tools.RunAgentSpec) (tools.RunAgentResult, error) {
		return tools.RunAgentResult{}, errors.New("all 2 subagent tasks failed; first error: boom")
	}}
	res, err := rt.runAdhocFlow(ctx, caller, false, adhocThreeStepSpec(), newAdhocRun(0, stub.fanOut))
	if err != nil {
		t.Fatalf("a failed run is a result, not a call error: %v", err)
	}
	if stub.rounds() != 1 {
		t.Fatalf("round two must not start after an all-failed round; rounds=%d", stub.rounds())
	}
	got := stepStatuses(res)
	if res.Status != tools.AdhocStatusFailed || got["scan"] != "failed" || got["triage"] != "skipped" || got["fix"] != "skipped" {
		t.Fatalf("want failed run with scan failed and the rest skipped, got %s %v", res.Status, got)
	}
	if !strings.Contains(res.Steps[0].Output, "boom") {
		t.Fatalf("the failed step must carry its error, got %q", res.Steps[0].Output)
	}
	run, _ := rt.db.GetFlowRun(context.Background(), res.RunID)
	if run.Status != db.FlowFailure {
		t.Fatalf("the FlowRun must be failed, got %q", run.Status)
	}
	drainSpawns(t, rt)
}

// TestAdhocFlowPartialFailureContinues: one failed leg is reported, not fatal —
// the run goes on, and the failure is visible in the step output only.
func TestAdhocFlowPartialFailureContinues(t *testing.T) {
	rt, caller, ctx := adhocFixture(t)
	stub := &adhocStub{reply: func(_ context.Context, round int, spec tools.RunAgentSpec) (tools.RunAgentResult, error) {
		res := answers(spec, "all clear")
		if round == 1 {
			res.FanOut[1] = tools.FanOutOutcome{Index: 1, Target: "reviewer", Error: "FAILED to start"}
		}
		return res, nil
	}}
	res, err := rt.runAdhocFlow(ctx, caller, false, adhocThreeStepSpec(), newAdhocRun(0, stub.fanOut))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := stepStatuses(res)
	// The failed leg's error text says "FAILED", but the branch matches only the
	// answers — so the default arm ends the run and the fix round is skipped.
	if res.Status != tools.AdhocStatusDone || got["scan"] != "done" || got["fix"] != "skipped" {
		t.Fatalf("want done with fix skipped, got %s %v", res.Status, got)
	}
	if !strings.Contains(res.Steps[0].Output, "FAILED: FAILED to start") {
		t.Fatalf("the failed leg must be reported in the step output, got %q", res.Steps[0].Output)
	}
	drainSpawns(t, rt)
}

// TestAdhocFlowRoundCap: a branch looping back re-runs a parallel step; the round
// cap stops it instead of the engine's step cap.
func TestAdhocFlowRoundCap(t *testing.T) {
	rt, caller, ctx := adhocFixture(t)
	spec := tools.AdhocFlowSpec{MaxRounds: 2, Steps: []tools.AdhocFlowStep{
		{ID: "try", Type: tools.AdhocStepParallel, Next: "check", Tasks: []tools.RunAgentTask{{Target: "explore", Task: "try"}}},
		{ID: "check", Type: tools.AdhocStepBranch, On: "try", MatchMode: "contains", Branches: []tools.AdhocFlowBranch{
			{Value: "again", Next: "try"},
			{Value: "", Next: ""},
		}},
	}}
	stub := &adhocStub{reply: func(_ context.Context, _ int, spec tools.RunAgentSpec) (tools.RunAgentResult, error) {
		return answers(spec, "again"), nil
	}}
	res, err := rt.runAdhocFlow(ctx, caller, false, spec, newAdhocRun(0, stub.fanOut))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stub.rounds() != 2 || res.Status != tools.AdhocStatusFailed || !strings.Contains(res.Error, "round cap (2)") {
		t.Fatalf("want the round cap to stop the third round, rounds=%d res=%+v", stub.rounds(), res)
	}
	if _, err := rt.runAdhocFlow(ctx, caller, false, tools.AdhocFlowSpec{MaxRounds: AdhocFlowMaxRounds + 1, Steps: spec.Steps}, newAdhocRun(0, stub.fanOut)); err == nil {
		t.Fatal("max_rounds above the ceiling must be refused")
	}
	drainSpawns(t, rt)
}

// TestAdhocFlowCancelledMidRound: cancelling the caller's turn while round two
// runs aborts the run cleanly — round two reads "cancelled", the run is no longer
// running, and nothing after it starts.
func TestAdhocFlowCancelledMidRound(t *testing.T) {
	rt, caller, base := adhocFixture(t)
	ctx, cancel := context.WithCancel(base)
	defer cancel()
	spec := adhocThreeStepSpec()
	spec.Steps[2].Next = "after"
	spec.Steps = append(spec.Steps, tools.AdhocFlowStep{ID: "after", Type: tools.AdhocStepEnd})

	started := make(chan struct{})
	stub := &adhocStub{reply: func(c context.Context, round int, spec tools.RunAgentSpec) (tools.RunAgentResult, error) {
		if round == 1 {
			return answers(spec, "FAIL"), nil
		}
		close(started)
		<-c.Done() // a leg still running when the turn is stopped
		return tools.RunAgentResult{}, c.Err()
	}}

	type out struct {
		res tools.AdhocFlowResult
		err error
	}
	done := make(chan out, 1)
	go func() {
		res, err := rt.runAdhocFlow(ctx, caller, false, spec, newAdhocRun(0, stub.fanOut))
		done <- out{res, err}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("round two never started")
	}
	cancel()
	var got out
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not stop after the caller's turn was cancelled")
	}
	if got.err != nil {
		t.Fatalf("a cancelled run is still a result: %v", got.err)
	}
	st := stepStatuses(got.res)
	if got.res.Status != tools.AdhocStatusCancelled || st["scan"] != "done" || st["fix"] != "cancelled" || st["after"] != "skipped" {
		t.Fatalf("want cancelled run (scan done, fix cancelled, after skipped), got %s %v", got.res.Status, st)
	}
	run, err := rt.db.GetFlowRun(context.Background(), got.res.RunID)
	if err != nil || run.Status == db.FlowRunning {
		t.Fatalf("a cancelled run must not be left running: %+v %v", run, err)
	}
	drainSpawns(t, rt)
}

// TestAdhocFlowRefusedBeforeAnyRow: an unknown leg target is caught before the
// hidden flow row or a FlowRun exists.
func TestAdhocFlowRefusedBeforeAnyRow(t *testing.T) {
	rt, caller, ctx := adhocFixture(t)
	spec := adhocThreeStepSpec()
	spec.Steps[2].Tasks[0].Target = "no-such-agent"
	stub := &adhocStub{reply: func(context.Context, int, tools.RunAgentSpec) (tools.RunAgentResult, error) {
		t.Fatal("no round may run for a refused plan")
		return tools.RunAgentResult{}, nil
	}}
	if _, err := rt.runAdhocFlow(ctx, caller, false, spec, newAdhocRun(0, stub.fanOut)); err == nil || !strings.Contains(err.Error(), "no-such-agent") {
		t.Fatalf("want an unknown-target refusal, got %v", err)
	}
	if runs, _ := rt.db.ListRunningFlowRuns(context.Background()); len(runs) != 0 {
		t.Fatalf("a refused plan must leave no run, got %d", len(runs))
	}
	drainSpawns(t, rt)
}

// TestAdhocFlowToolThroughRealFanOut drives the tool end to end on the real
// runAgentFanOut: the parent session does not exist, so every leg fails at child
// session creation — the all-failed round must stop the run there.
func TestAdhocFlowToolThroughRealFanOut(t *testing.T) {
	rt, caller, _ := adhocFixture(t)
	var n int32
	st := delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n}
	ctx := WithSessionID(context.WithValue(context.Background(), delegStateKey{}, st), "SES-nope")
	ctx = rt.withRunAdhocFlow(ctx, caller, nil, false)

	args, _ := json.Marshal(map[string]any{"steps": []map[string]any{
		{"id": "scan", "type": "parallel", "next": "fix", "tasks": []map[string]any{{"target": "explore", "task": "look"}}},
		{"id": "fix", "type": "parallel", "tasks": []map[string]any{{"target": "coder", "task": "fix"}}},
	}})
	out, err := tools.NewRunAdhocFlowTool().Call(ctx, args)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var res tools.AdhocFlowResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("result must be JSON: %v\n%s", err, out)
	}
	got := stepStatuses(res)
	if res.Status != tools.AdhocStatusFailed || got["scan"] != "failed" || got["fix"] != "skipped" {
		t.Fatalf("want scan failed and fix skipped, got %s %v", res.Status, got)
	}
	if !strings.Contains(res.Steps[0].Output, "all 1 subagent tasks failed") {
		t.Fatalf("the all-failed error should surface, got %q", res.Steps[0].Output)
	}
	drainSpawns(t, rt)
}
