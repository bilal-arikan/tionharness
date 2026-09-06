package agent

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// aggregateOutcomes builds three successful legs to aggregate.
func aggregateOutcomes() []tools.FanOutOutcome {
	return []tools.FanOutOutcome{
		{Index: 0, Target: "explore", AgentName: "A", Reply: "yes"},
		{Index: 1, Target: "explore", AgentName: "B", Reply: "no"},
		{Index: 2, Target: "explore", AgentName: "C", Reply: "YES"},
	}
}

// TestAggregateLeavesCollectingStrategiesUntouched is the regression guard for the
// whole card: "all" and "first-success" must come out of aggregation bit for bit
// as they went in — no winner elected, no agreement counted, nothing rewritten.
// An unset strategy takes the same path, since specs built in-process may leave it
// empty and must not be judged by accident.
func TestAggregateLeavesCollectingStrategiesUntouched(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	caller, _ := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Caller", Provider: "claude-cli"})

	for _, strategy := range []string{"", tools.StrategyAll, tools.StrategyFirstSuccess} {
		outcomes := aggregateOutcomes()
		before := aggregateOutcomes()
		spec := tools.RunAgentSpec{Strategy: strategy, Tasks: []tools.RunAgentTask{{Task: "a"}, {Task: "b"}, {Task: "c"}}}
		if err := rt.aggregateFanOut(context.Background(), caller, nil, false, spec, outcomes); err != nil {
			t.Fatalf("strategy %q must aggregate nothing and fail at nothing: %v", strategy, err)
		}
		if !reflect.DeepEqual(outcomes, before) {
			t.Fatalf("strategy %q rewrote its outcomes: %+v (was %+v)", strategy, outcomes, before)
		}
	}
	drainSpawns(t, rt)
}

// TestAggregateMajorityMarksTheWinnerAndEveryVote: the caller decides how much to
// trust the answer from the vote counts, so they must be filled in for every leg,
// not only the winner's.
func TestAggregateMajorityMarksTheWinnerAndEveryVote(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	caller, _ := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Caller", Provider: "claude-cli"})

	outcomes := aggregateOutcomes()
	spec := tools.RunAgentSpec{Strategy: tools.StrategyMajority}
	if err := rt.aggregateFanOut(context.Background(), caller, nil, false, spec, outcomes); err != nil {
		t.Fatalf("two legs answered the same: %v", err)
	}
	if !outcomes[0].Winner || outcomes[1].Winner || outcomes[2].Winner {
		t.Fatalf("exactly the lowest-index member of the winning class wins, got %+v", outcomes)
	}
	if outcomes[0].Agreement != 2 || outcomes[2].Agreement != 2 || outcomes[1].Agreement != 1 {
		t.Fatalf("every leg must carry its own vote count, got %+v", outcomes)
	}
}

// TestAggregateMajorityWithoutAgreementFailsTheCall: no silent fallback to "the
// first leg" — the failure has to reach the caller so it can re-run differently.
func TestAggregateMajorityWithoutAgreementFailsTheCall(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	caller, _ := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Caller", Provider: "claude-cli"})

	outcomes := []tools.FanOutOutcome{
		{Index: 0, AgentName: "A", Reply: "one"},
		{Index: 1, AgentName: "B", Reply: "two"},
	}
	err := rt.aggregateFanOut(context.Background(), caller, nil, false,
		tools.RunAgentSpec{Strategy: tools.StrategyMajority}, outcomes)
	if err == nil || !strings.Contains(err.Error(), "no majority to report") {
		t.Fatalf("expected a no-majority error, got %v", err)
	}
	if outcomes[0].Winner || outcomes[1].Winner {
		t.Fatalf("a failed vote must elect nobody, got %+v", outcomes)
	}
}

// TestAggregateRefusesAnUnknownStrategy: the tool validates the enum, but a silent
// default here would let a future strategy behave like "all" while its name
// promised a winner.
func TestAggregateRefusesAnUnknownStrategy(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	caller, _ := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Caller", Provider: "claude-cli"})

	err := rt.aggregateFanOut(context.Background(), caller, nil, false,
		tools.RunAgentSpec{Strategy: "consensus"}, aggregateOutcomes())
	if err == nil || !strings.Contains(err.Error(), "unknown fan-out strategy") {
		t.Fatalf("expected an unknown-strategy error, got %v", err)
	}
}

// TestReviewerFailureFailsTheCall: reviewer-selects promised exactly one answer,
// so degrading to "here are all the candidates" would hand back a pile the caller
// would read as findings. The error must instead say how many candidates were lost
// and how to get them.
func TestReviewerFailureFailsTheCall(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	caller, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "claude-cli"})

	// "SES-nope" does not exist, so the reviewer run fails at child-session
	// creation — after its guards, before any provider call.
	var n int32
	st := delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n}
	rctx := WithSessionID(context.WithValue(ctx, delegStateKey{}, st), "SES-nope")

	outcomes := aggregateOutcomes()
	spec := tools.RunAgentSpec{
		Strategy: tools.StrategyReviewerSelects,
		Tasks:    []tools.RunAgentTask{{Task: "a"}, {Task: "b"}, {Task: "c"}},
	}
	err := rt.aggregateFanOut(rctx, caller, nil, false, spec, outcomes)
	if err == nil {
		t.Fatal("a reviewer that cannot run must fail the call")
	}
	if !strings.Contains(err.Error(), "the reviewer run failed") ||
		!strings.Contains(err.Error(), "3 candidate answers are not returned") {
		t.Fatalf("the error must name the loss and the way out: %v", err)
	}
	for _, o := range outcomes {
		if o.Winner {
			t.Fatalf("no leg may be elected when the judge never ran: %+v", o)
		}
	}
	drainSpawns(t, rt)
}
