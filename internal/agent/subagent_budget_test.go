package agent

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// budgetRunnerFor wires a runner whose chain position carries the shared counter n
// and whose context names parentSessionID as the calling session — runAgent refuses
// without one, and the budget spend sits below that check.
func budgetRunnerFor(t *testing.T, rt *Runtime, caller db.Agent, n *int32, parentSessionID string) func(tools.RunAgentSpec) (tools.RunAgentResult, error) {
	t.Helper()
	st := delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: n}
	ctx := WithSessionID(context.WithValue(context.Background(), delegStateKey{}, st), parentSessionID)
	ctx = rt.withRunAgent(ctx, caller, nil, false)
	run := tools.RunAgentFrom(ctx)
	if run == nil {
		t.Fatal("expected a run-agent runner in context")
	}
	return func(spec tools.RunAgentSpec) (tools.RunAgentResult, error) { return run(ctx, spec) }
}

// TestDelegationBudgetRefundedWhenChildSessionFails: a run that never started spent
// no provider tokens, so it must not shrink the turn's delegation budget — the
// child row was never created, so nothing ran.
func TestDelegationBudgetRefundedWhenChildSessionFails(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	caller, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "claude-cli"})
	target, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "claude-cli"})

	var n int32
	// A parent session id that does not exist: CreateChildSession rejects it, after
	// the budget spend.
	run := budgetRunnerFor(t, rt, caller, &n, "SES-nope")
	if _, err := run(tools.RunAgentSpec{Target: target.Name, Task: "do x"}); err == nil {
		t.Fatal("expected the child session creation to fail")
	}
	if got := atomic.LoadInt32(&n); got != 0 {
		t.Fatalf("a child session that was never created must refund its budget unit; counter is %d", got)
	}
}

// TestDelegationBudgetSpendAndRefundAreExactInverses guards the helper itself,
// including that the over-cap probe leaves no residue behind.
func TestDelegationBudgetSpendAndRefundAreExactInverses(t *testing.T) {
	const cap32 = 3
	var n int32
	st := delegState{calls: &n}

	for i := 1; i <= cap32; i++ {
		if !st.spend(cap32) {
			t.Fatalf("spend %d of %d was refused", i, cap32)
		}
		if got := atomic.LoadInt32(&n); got != int32(i) {
			t.Fatalf("after spend %d the counter is %d", i, got)
		}
	}
	if st.spend(cap32) {
		t.Fatal("spending past the cap must be refused")
	}
	if got := atomic.LoadInt32(&n); got != cap32 {
		t.Fatalf("the over-cap probe must undo its own add; counter is %d", got)
	}
	st.refund()
	if got := atomic.LoadInt32(&n); got != cap32-1 {
		t.Fatalf("refund must return exactly one unit; counter is %d", got)
	}

	// A turn with no counter (no chain position) is unbounded and must not panic.
	empty := delegState{}
	if !empty.spend(cap32) {
		t.Fatal("a counterless chain position must always be allowed to spend")
	}
	empty.refund()
}

// TestAdhocDelegationCeiling pins the ad-hoc budget cap: DelegationMaxCalls ×
// max_rounds, hard-capped at AdhocFlowMaxDelegationCalls, never below the
// ordinary per-turn cap.
func TestAdhocDelegationCeiling(t *testing.T) {
	cases := []struct{ calls, rounds, want int }{
		{8, 1, 8},
		{8, 2, 16},
		{8, 3, AdhocFlowMaxDelegationCalls}, // 24 exactly
		{10, 3, AdhocFlowMaxDelegationCalls},
		{50, 3, 50}, // a workspace already above the hard cap keeps its own cap
	}
	for _, c := range cases {
		if got := adhocDelegationCeiling(c.calls, c.rounds); got != c.want {
			t.Errorf("ceiling(%d calls, %d rounds) = %d, want %d", c.calls, c.rounds, got, c.want)
		}
	}
}

// TestAdhocCeilingHoldsUnderConcurrentLegs: legs of every round spend the SAME
// shared counter against the widened ceiling, so however many run at once,
// exactly `ceiling` of them get a unit — the cap is not reset per round.
func TestAdhocCeilingHoldsUnderConcurrentLegs(t *testing.T) {
	var n int32
	st := delegState{depth: 0, visited: map[string]bool{}, calls: &n}
	ceiling := adhocDelegationCeiling(DefaultMaxDelegationCalls, AdhocFlowMaxRounds)
	ctx := withDelegationCeiling(context.Background(), ceiling)

	const legs = 64
	var granted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < legs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if st.spend(delegationCeiling(ctx, DefaultMaxDelegationCalls)) {
				granted.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := int(granted.Load()); got != ceiling {
		t.Fatalf("%d concurrent legs against a ceiling of %d: %d were granted", legs, ceiling, got)
	}
	if got := atomic.LoadInt32(&n); int(got) != ceiling {
		t.Fatalf("the shared counter must stop at the ceiling; counter is %d", got)
	}
}

// TestRunAgentHonoursAdhocCeiling: runAgent reads its cap through
// delegationCeiling, so a turn that already spent the ordinary cap is refused on
// the plain path but may continue inside an ad-hoc run.
func TestRunAgentHonoursAdhocCeiling(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetDelegationLimits(0, 2)
	ctx := context.Background()
	caller, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "claude-cli"})

	n := int32(2) // the ordinary cap is already spent
	st := delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n}
	base := WithSessionID(context.WithValue(ctx, delegStateKey{}, st), "SES-nope")

	_, err := rt.runAgent(base, caller, nil, false, tools.RunAgentSpec{Target: "explore", Task: "x"})
	if err == nil || !strings.Contains(err.Error(), "budget (2 per turn) exhausted") {
		t.Fatalf("the plain path must refuse at the ordinary cap, got %v", err)
	}
	widened := withDelegationCeiling(base, adhocDelegationCeiling(2, 2))
	_, err = rt.runAgent(widened, caller, nil, false, tools.RunAgentSpec{Target: "explore", Task: "x"})
	if err == nil || strings.Contains(err.Error(), "budget") {
		t.Fatalf("inside an ad-hoc run the budget must admit the call (it then fails on the missing session), got %v", err)
	}
	if got := atomic.LoadInt32(&n); got != 2 {
		t.Fatalf("the failed child session must refund its unit; counter is %d", got)
	}
	drainSpawns(t, rt)
}
