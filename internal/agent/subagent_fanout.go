package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// runAgentFanOut executes the multi-task form of run_subagent: several subagents
// from one call, aggregated by the requested strategy.
//
// Each leg goes through the ordinary single-task runAgent, so every guard the
// single path enforces — depth, cycle, the shared per-turn budget counter, child
// session creation, artifact ownership — applies unchanged and is not duplicated
// here. This function only adds concurrency limiting, cancellation and ordering.
func (r *Runtime) runAgentFanOut(ctx context.Context, caller db.Agent, parentReq *providers.Request, autonomous bool, spec tools.RunAgentSpec) (tools.RunAgentResult, error) {
	legs := spec.Tasks
	limit := spec.MaxConcurrency
	if limit <= 0 {
		limit = tools.DefaultFanOutConcurrency
	}
	if limit > len(legs) {
		limit = len(legs)
	}

	// first-success needs its own cancellable scope: cancelling the CALLER's ctx
	// would take down the turn that asked for the fan-out.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	outcomes := make([]tools.FanOutOutcome, len(legs))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var once sync.Once

	for i, leg := range legs {
		wg.Add(1)
		go func(i int, leg tools.RunAgentTask) {
			defer wg.Done()
			outcomes[i] = tools.FanOutOutcome{Index: i, Target: leg.Target}

			// A leg that never gets a slot before a first-success winner appears is
			// SKIPPED, not failed: it produced no verdict, and reporting it as a
			// failure would tell the caller its route was tried and did not work.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-runCtx.Done():
				outcomes[i].Skipped = true
				return
			}
			if runCtx.Err() != nil {
				outcomes[i].Skipped = true
				return
			}

			res, err := r.runAgent(runCtx, caller, parentReq, autonomous, legSpec(spec, leg))
			if err != nil {
				// Cancellation of a leg that was already running is the same event as
				// never starting it — the winner made this route moot, and the caller
				// must not read it as evidence the route failed.
				if runCtx.Err() != nil && ctx.Err() == nil {
					outcomes[i].Skipped = true
					return
				}
				outcomes[i].Error = err.Error()
				return
			}
			outcomes[i].AgentName = res.AgentName
			outcomes[i].Reply = res.Reply
			outcomes[i].Artifacts = res.Artifacts
			if spec.Strategy == tools.StrategyFirstSuccess {
				once.Do(cancel)
			}
		}(i, leg)
	}
	wg.Wait()

	// The caller's own turn being cancelled is a real abort, not a fan-out result.
	if ctx.Err() != nil {
		return tools.RunAgentResult{}, ctx.Err()
	}

	succeeded := 0
	for _, o := range outcomes {
		if o.Error == "" && !o.Skipped {
			succeeded++
		}
	}
	// Every leg failing is a failed call: there is no partial answer to hand back,
	// and returning "here are 3 errors" as a success would have the model treat
	// them as findings. One success is enough to report — that is what partial
	// failure means.
	if succeeded == 0 {
		return tools.RunAgentResult{}, fmt.Errorf("all %d subagent tasks failed; first error: %s", len(legs), firstError(outcomes))
	}
	r.logger.Info("subagent fan-out", "tasks", len(legs), "strategy", spec.Strategy, "succeeded", succeeded, "concurrency", limit)
	return tools.RunAgentResult{FanOut: outcomes, Strategy: spec.Strategy}, nil
}

// legSpec turns one fan-out leg into the single-task spec runAgent understands.
// The fan-out axes are dropped so a leg can never recurse into another fan-out
// through the same call.
func legSpec(base tools.RunAgentSpec, leg tools.RunAgentTask) tools.RunAgentSpec {
	return tools.RunAgentSpec{
		Target:       leg.Target,
		Task:         leg.Task,
		Context:      leg.Context,
		Model:        leg.Model,
		Objective:    leg.Objective,
		OutputFormat: leg.OutputFormat,
		Boundaries:   leg.Boundaries,
	}
}

// firstError returns the first reported leg error, for the all-failed message.
func firstError(outcomes []tools.FanOutOutcome) string {
	for _, o := range outcomes {
		if o.Error != "" {
			return o.Error
		}
	}
	return "unknown"
}
