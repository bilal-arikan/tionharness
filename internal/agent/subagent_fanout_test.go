package agent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestFanOutAllLegsFailingIsACallFailure: with no leg succeeding there is no
// partial answer to hand back, and returning a list of errors as a SUCCESS would
// have the calling model read them as findings.
func TestFanOutAllLegsFailingIsACallFailure(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	caller, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "claude-cli"})
	target, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "claude-cli"})

	var n int32
	// "SES-nope" does not exist, so every leg fails at child-session creation —
	// after its guards, before any provider call.
	run := budgetRunnerFor(t, rt, caller, &n, "SES-nope")
	_, err := run(tools.RunAgentSpec{
		Target:   target.Name,
		Strategy: tools.StrategyAll,
		Tasks: []tools.RunAgentTask{
			{Target: target.Name, Task: "a"},
			{Target: target.Name, Task: "b"},
			{Target: target.Name, Task: "c"},
		},
	})
	if err == nil {
		t.Fatal("expected the all-failed fan-out to be an error")
	}
	if !strings.Contains(err.Error(), "all 3 subagent tasks failed") {
		t.Fatalf("error should name how many legs failed: %v", err)
	}
	drainSpawns(t, rt)
}

// TestFanOutLegsShareTheTurnBudget: a fan-out must not be a way around the
// per-turn delegation cap — the legs spend the SAME counter the single-task path
// spends, and a leg refused by the cap is a failed leg, not a silent skip.
func TestFanOutLegsShareTheTurnBudget(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetDelegationLimits(0, 2)
	ctx := context.Background()
	caller, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "claude-cli"})
	target, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "claude-cli"})

	var n int32
	run := budgetRunnerFor(t, rt, caller, &n, "SES-nope")
	_, err := run(tools.RunAgentSpec{
		Target: target.Name,
		// Four legs against a budget of two: the fan-out itself must not buy extra
		// headroom.
		Tasks: []tools.RunAgentTask{
			{Target: target.Name, Task: "a"},
			{Target: target.Name, Task: "b"},
			{Target: target.Name, Task: "c"},
			{Target: target.Name, Task: "d"},
		},
		MaxConcurrency: 1,
	})
	if err == nil {
		t.Fatal("expected the fan-out to fail (no legs can succeed here)")
	}
	if got := atomic.LoadInt32(&n); got != 0 {
		t.Fatalf("legs that never created a child session must refund; counter is %d", got)
	}
	drainSpawns(t, rt)
}

// TestFanOutDispatchSkipsTheCallersOwnGuards: the fan-out call runs no subagent of
// its own, so charging it depth/budget would bill the turn for a dispatcher. Each
// leg re-enters runAgent and is guarded there.
func TestFanOutDispatchSkipsTheCallersOwnGuards(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	caller, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "claude-cli"})
	target, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "claude-cli"})

	// A chain position already AT the depth limit: a single-task call is refused
	// here, and each fan-out leg must be refused for the same reason — but by the
	// leg's own guard, so the failure is reported per leg.
	var n int32
	st := delegState{depth: DefaultMaxDelegationDepth, visited: map[string]bool{caller.ID: true}, calls: &n}
	rctx := WithSessionID(context.WithValue(context.Background(), delegStateKey{}, st), "SES-nope")
	rctx = rt.withRunAgent(rctx, caller, nil, false)
	run := tools.RunAgentFrom(rctx)

	_, err := run(rctx, tools.RunAgentSpec{
		Target: target.Name,
		Tasks:  []tools.RunAgentTask{{Target: target.Name, Task: "a"}},
	})
	if err == nil || !strings.Contains(err.Error(), "depth limit") {
		t.Fatalf("the leg's own depth guard should surface, got %v", err)
	}
	drainSpawns(t, rt)
}

// TestLegSpecDropsFanOutAxes: a leg must not be able to recurse into another
// fan-out through the same call — that would multiply the tree without ever
// passing the dispatcher's own accounting.
func TestLegSpecDropsFanOutAxes(t *testing.T) {
	base := tools.RunAgentSpec{
		Target:         "explore",
		Strategy:       tools.StrategyFirstSuccess,
		MaxConcurrency: 8,
		RetryOf:        "SES9",
		Tasks:          []tools.RunAgentTask{{Task: "x"}},
	}
	leg := legSpec(base, tools.RunAgentTask{Target: "reviewer", Task: "check", Model: "m"})
	if len(leg.Tasks) != 0 || leg.Strategy != "" || leg.MaxConcurrency != 0 {
		t.Fatalf("leg spec must carry no fan-out state, got %+v", leg)
	}
	if leg.RetryOf != "" {
		t.Fatalf("leg spec must not inherit retry lineage, got %+v", leg)
	}
	if leg.Target != "reviewer" || leg.Task != "check" || leg.Model != "m" {
		t.Fatalf("leg spec should carry the leg's own axes, got %+v", leg)
	}
}

// TestFirstErrorReportsTheFirstFailure: the all-failed message quotes one error,
// and it must be the first leg's — the caller reads legs in order.
func TestFirstErrorReportsTheFirstFailure(t *testing.T) {
	got := firstError([]tools.FanOutOutcome{
		{Index: 0, Skipped: true},
		{Index: 1, Error: "boom"},
		{Index: 2, Error: "later"},
	})
	if got != "boom" {
		t.Fatalf("expected the first reported error, got %q", got)
	}
	if got := firstError([]tools.FanOutOutcome{{Index: 0, Skipped: true}}); got != "unknown" {
		t.Fatalf("with no error at all the message must still be well-formed, got %q", got)
	}
}
