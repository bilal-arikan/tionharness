package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
)

// Decider site "stall-judge": the phantom-spawn judge as one yes/no question.
// It asks exactly what the stall-judge system agent is asked
// (prompts/defaults/stall-judge.md), so a shadow comparison measures the model,
// not two different questions.

const stallQuestionKey = "stalled"

// stallDecisionRequest builds the decision request for a coordinator message.
func stallDecisionRequest(text string) decider.Request {
	return decider.Request{
		State: "Coordinator's latest message:\n\n" + truncateRunes(text, 4000),
		Questions: map[string]decider.Question{
			stallQuestionKey: decider.Noul(
				"This coordinator turn made NO worker-spawn or coordination tool call, and no worker is running under it. "+
					"Does its latest message claim (in any language) to have just started, spawned, opened or launched workers, branches or sub-tasks, "+
					"or report them as running or in progress?",
				"It narrates delegation as done or underway, e.g. \"spawned 3 workers\", \"Round 5 opened - 2 arms\", \"SES144 açıldı\", \"[running]\".",
				"It plainly concludes, reports already-finished work, asks the user a question, or narrates only its own non-delegated actions.",
			),
		},
	}
}

// stallVerdict renders a boolean verdict in the ledger's vocabulary.
func stallVerdict(stalled bool) string {
	if stalled {
		return "stalled"
	}
	return "ok"
}

// decideCoordinatorStalled is the on-mode path: the decider's verdict replaces
// the LLM judge. ok=false means "no verdict, run the LLM judge" (site not on,
// or the decider failed).
func (r *Runtime) decideCoordinatorStalled(ctx context.Context, agent db.Agent, text string) (stalled, ok bool) {
	if r.deciderMode(decider.SiteStallJudge) != decider.ModeOn {
		return false, false
	}
	resp, err := r.decide(ctx, decider.SiteStallJudge, agent, stallDecisionRequest(text))
	rec := decider.NewRecord(decider.SiteStallJudge, decider.ModeOn, resp, err)
	if err != nil {
		if !decisionOff(err) {
			r.logger.Info("stall decision unavailable; using the LLM judge", "agent", agent.ID, "error", err)
			r.logDecision(rec)
		}
		return false, false
	}
	a := resp.Answers[stallQuestionKey]
	stalled = a.Yes(r.deciderThreshold(decider.SiteStallJudge))
	rec.Outcome, rec.Strength, rec.Applied = stallVerdict(stalled), a.Probability, true
	r.logDecision(rec)
	return stalled, true
}

// shadowCoordinatorStalled compares the LLM judge's verdict with the decider's,
// in the background, when the site is in shadow mode.
func (r *Runtime) shadowCoordinatorStalled(ctx context.Context, agent db.Agent, text string, llmStalled bool) {
	threshold := r.deciderThreshold(decider.SiteStallJudge)
	r.shadowDecision(ctx, decider.SiteStallJudge, agent, stallDecisionRequest(text), stallVerdict(llmStalled), SessionIDFrom(ctx),
		func(resp *decider.Response) (string, float64) {
			a := resp.Answers[stallQuestionKey]
			return stallVerdict(a.Yes(threshold)), a.Probability
		})
}
