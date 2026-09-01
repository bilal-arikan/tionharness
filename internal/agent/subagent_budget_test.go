package agent

import (
	"context"
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
