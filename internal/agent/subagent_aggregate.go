package agent

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// reviewerProfileID is the built-in profile that judges a "reviewer-selects"
// fan-out. It is read-only by construction (Read/LS/Glob/Grep), which is exactly
// what a judge should be: it reads the candidates and names one.
const reviewerProfileID = "reviewer"

// aggregateFanOut applies a rank-and-pick strategy to a finished fan-out, marking
// the winning leg in the outcomes in place.
//
// The collecting strategies return immediately and untouched. That is the whole
// contract of this function: "all" and "first-success" behave exactly as they did
// before rank-and-pick existed, because nothing on their path runs.
func (r *Runtime) aggregateFanOut(ctx context.Context, caller db.Agent, parentReq *providers.Request, autonomous bool, spec tools.RunAgentSpec, outcomes []tools.FanOutOutcome) error {
	switch spec.Strategy {
	case "", tools.StrategyAll, tools.StrategyFirstSuccess:
		// An unset strategy is the "all" default buildFanOutSpec fills in for calls
		// that come through the tool; specs built in-process may leave it empty.
		return nil
	case tools.StrategyMajority:
		winner, agreement, err := tools.MajorityWinner(outcomes)
		if err != nil {
			return err
		}
		for i := range outcomes {
			outcomes[i].Agreement = agreement[i]
		}
		outcomes[winner].Winner = true
		return nil
	case tools.StrategyReviewerSelects:
		winner, err := r.reviewerSelect(ctx, caller, parentReq, autonomous, spec, outcomes)
		if err != nil {
			return err
		}
		outcomes[winner].Winner = true
		return nil
	default:
		// Unreachable through the tool — buildFanOutSpec validates the enum — but a
		// silent default here would let a future strategy quietly behave like "all"
		// while its name promised something else.
		return fmt.Errorf("unknown fan-out strategy %q", spec.Strategy)
	}
}

// reviewerSelect runs the judging turn of "reviewer-selects" and returns the index
// of the leg the reviewer picked.
//
// The reviewer is an ordinary subagent run: it goes through runAgent, so it is
// charged to the same per-turn delegation budget and bounded by the same guards
// as any leg. Charging it is correct — it really is one more agent run — and it
// means a fan-out that only just fits the budget may lose its judge, which the
// error below states plainly instead of hiding.
//
// DESIGN — a failing reviewer fails the CALL. Falling back to "here are all the
// candidates" would hand back N answers under a strategy that promised exactly
// one, and a caller (or a prompt) built around a single answer would misread the
// pile as findings. The N candidate runs are lost, which is the price of not
// lying about the contract; the error says how many candidates there were so the
// caller can decide to re-run with "all" and judge them itself.
func (r *Runtime) reviewerSelect(ctx context.Context, caller db.Agent, parentReq *providers.Request, autonomous bool, spec tools.RunAgentSpec, outcomes []tools.FanOutOutcome) (int, error) {
	res, err := r.runAgent(ctx, caller, parentReq, autonomous, tools.RunAgentSpec{
		Target:  reviewerProfileID,
		Task:    tools.BuildReviewerPrompt(spec.Tasks, outcomes),
		Context: "isolated",
		// The judge runs on the same model the caller chose for the candidates: a
		// separate knob would be one more axis to get wrong, and a judge weaker than
		// the candidates it ranks is not worth the call.
		Model:        spec.Model,
		Objective:    "Pick the single best of the candidate answers.",
		OutputFormat: "the candidate's number and nothing else",
	})
	if err != nil {
		return 0, fmt.Errorf(
			"strategy %q: the reviewer run failed (%v); the %d candidate answers are not returned — re-run with strategy %q to read and judge them yourself",
			tools.StrategyReviewerSelects, err, countAnsweredLegs(outcomes), tools.StrategyAll)
	}
	idx, err := tools.ParseReviewerChoice(res.Reply, outcomes)
	if err != nil {
		return 0, fmt.Errorf("strategy %q: %w", tools.StrategyReviewerSelects, err)
	}
	return idx, nil
}

// countAnsweredLegs is how many legs produced an answer, for the reviewer-failure
// message.
func countAnsweredLegs(outcomes []tools.FanOutOutcome) int {
	n := 0
	for _, o := range outcomes {
		if o.Error == "" && !o.Skipped {
			n++
		}
	}
	return n
}
