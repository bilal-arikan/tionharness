package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// adhocRun is the per-call state of one run_adhoc_flow execution, carried on the
// context into the engine's fan-out hook. It binds the fan-out to the calling
// agent (the engine only knows node ids), counts rounds against the cap, and
// records each parallel step's leg outcomes — per-leg results come from here,
// never from the engine trace, which holds one entry per fan-out node.
type adhocRun struct {
	maxRounds int
	rounds    atomic.Int32

	// fanOut runs one round's legs. In production it is the existing
	// runAgentFanOut bound to the caller, so legSpec, the depth/cycle guards and
	// the shared per-turn budget all apply unchanged. Tests substitute a stub.
	fanOut func(ctx context.Context, spec tools.RunAgentSpec) (tools.RunAgentResult, error)

	mu    sync.Mutex
	steps map[string]adhocStepRecord // node id -> its latest execution
}

// adhocStepRecord is what one parallel step's latest execution produced.
type adhocStepRecord struct {
	runs     int
	outcomes []tools.FanOutOutcome
	err      error
}

func newAdhocRun(maxRounds int, fanOut func(context.Context, tools.RunAgentSpec) (tools.RunAgentResult, error)) *adhocRun {
	return &adhocRun{maxRounds: maxRounds, fanOut: fanOut, steps: map[string]adhocStepRecord{}}
}

// record stores a parallel step's latest outcome.
func (a *adhocRun) record(nodeID string, outcomes []tools.FanOutOutcome, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rec := a.steps[nodeID]
	rec.runs++
	rec.outcomes = outcomes
	rec.err = err
	a.steps[nodeID] = rec
}

// step returns a parallel step's latest record, false when it never ran.
func (a *adhocRun) step(nodeID string) (adhocStepRecord, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rec, ok := a.steps[nodeID]
	return rec, ok
}

// adhocRunKey keys the *adhocRun on a context.
type adhocRunKey struct{}

func withAdhocRun(ctx context.Context, a *adhocRun) context.Context {
	return context.WithValue(ctx, adhocRunKey{}, a)
}

func adhocRunFrom(ctx context.Context) *adhocRun {
	a, _ := ctx.Value(adhocRunKey{}).(*adhocRun)
	return a
}

// RunFanOutNode implements orchestration.FanOutRunner: it runs a fan-out agent
// node's legs as ONE run_subagent fan-out (strategy "all") and returns the
// answering legs' replies as the node output. Every leg goes through
// runAgentFanOut → legSpec, so a leg can never fan out itself; repeated fan-out
// only happens at the graph level, one round per node execution.
//
// Fan-out nodes exist only inside run_adhoc_flow: a flow run without the ad-hoc
// binding (a saved flow edited by hand, or an ad-hoc run revived after a restart)
// fails the node explicitly instead of guessing a caller.
func (f flowRunner) RunFanOutNode(ctx context.Context, legs []orchestration.Leg) (string, error) {
	run := adhocRunFrom(ctx)
	if run == nil {
		return "", fmt.Errorf("fan-out agent nodes run only inside run_adhoc_flow (no caller bound to this run)")
	}
	nodeID := orchestration.NodeIDFromContext(ctx)
	if n := int(run.rounds.Add(1)); n > run.maxRounds {
		err := fmt.Errorf("round cap (%d) reached before step %q; raise max_rounds or shorten the plan", run.maxRounds, nodeID)
		run.record(nodeID, nil, err)
		return "", err
	}
	tasks := make([]tools.RunAgentTask, len(legs))
	for i, l := range legs {
		tasks[i] = tools.RunAgentTask{
			Target:       l.Target,
			Task:         l.Task,
			Context:      l.Context,
			Model:        l.Model,
			Objective:    l.Objective,
			OutputFormat: l.OutputFormat,
			Boundaries:   l.Boundaries,
		}
	}
	res, err := run.fanOut(ctx, tools.RunAgentSpec{Tasks: tasks, Strategy: tools.StrategyAll})
	run.record(nodeID, res.FanOut, err)
	if err != nil {
		// All legs failing (or the caller's turn being cancelled) is a node error:
		// the engine stops here, so the next round never starts on nothing.
		return "", err
	}
	return adhocNodeOutput(res.FanOut), nil
}

// adhocNodeOutput is the engine-visible output of a fan-out node — what a branch
// matches and what later legs read as {{last}} / {{node.<id>}}. It carries only
// the ANSWERS: a failed leg's error text would otherwise feed a "contains" match
// (an error saying "FAILED" matching an arm for "FAIL"). Failures stay visible in
// the tool result, which is built from the recorded outcomes. With exactly one
// leg the reply is passed verbatim so "equals" / "json_field" routing works.
func adhocNodeOutput(outcomes []tools.FanOutOutcome) string {
	if len(outcomes) == 1 {
		return outcomes[0].Reply
	}
	var b strings.Builder
	for _, o := range outcomes {
		if o.Error != "" || o.Skipped {
			continue
		}
		fmt.Fprintf(&b, "[%d] %s:\n%s\n\n", o.Index+1, o.Target, o.Reply)
	}
	return strings.TrimSpace(b.String())
}

// delegationCeilingKey carries a widened per-turn delegation cap on a context.
type delegationCeilingKey struct{}

// withDelegationCeiling widens the per-turn subagent budget cap for everything
// run under ctx (nested subagent runs inherit it through their child context).
// Only the CAP moves: the counter itself stays the turn's shared one.
func withDelegationCeiling(ctx context.Context, ceiling int) context.Context {
	return context.WithValue(ctx, delegationCeilingKey{}, ceiling)
}

// delegationCeiling returns the per-turn delegation cap in force for ctx: the
// widened ad-hoc ceiling when one is set, otherwise def.
func delegationCeiling(ctx context.Context, def int) int {
	if c, ok := ctx.Value(delegationCeilingKey{}).(int); ok && c > 0 {
		return c
	}
	return def
}

// adhocDelegationCeiling is the budget cap for one ad-hoc run:
// DelegationMaxCalls × maxRounds, hard-capped at AdhocFlowMaxDelegationCalls, and
// never below the ordinary per-turn cap (widening must not shrink a workspace
// that already allows more than the hard cap).
func adhocDelegationCeiling(maxCalls, maxRounds int) int {
	c := min(maxCalls*maxRounds, AdhocFlowMaxDelegationCalls)
	if c < maxCalls {
		c = maxCalls
	}
	return c
}
