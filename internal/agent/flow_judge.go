package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

// Decider site "flow-judge": flowRunner implements orchestration.JudgeRunner, so
// branch and loop nodes in the "judge" match mode are decided by the decision
// model. Errors are returned to the engine, which falls back to the default arm
// (branch) or keeps looping up to the cap (loop).

const (
	flowArmKey       = "arm"
	flowConditionKey = "holds"
	// flowJudgeValueRunes bounds the judged output. The decider trims further
	// to its model's context; this keeps a runaway output from being redacted
	// and sent in full first.
	flowJudgeValueRunes = 24000
)

// JudgeBranch implements orchestration.JudgeRunner.
func (f flowRunner) JudgeBranch(ctx context.Context, req orchestration.JudgeRequest) (int, float64, error) {
	question := strings.TrimSpace(req.Question)
	if question == "" {
		question = "Which option best describes this output?"
	}
	options := make(map[string]string, len(req.Options))
	for i, o := range req.Options {
		options[flowArmOption(i)] = o
	}
	resp, err := f.rt.decide(ctx, decider.SiteFlowJudge, f.judgeCaller(ctx, req.AgentID), decider.Request{
		State:     truncateRunes(req.Value, flowJudgeValueRunes),
		Questions: map[string]decider.Question{flowArmKey: decider.Choice(question, options)},
	})
	rec := decider.NewRecord(decider.SiteFlowJudge, decider.ModeOn, resp, err)
	rec.Ref = flowRunIDFromContext(ctx)
	if err != nil {
		if !decisionOff(err) {
			f.rt.logDecision(rec)
		}
		return -1, 0, err
	}
	a := resp.Answers[flowArmKey]
	pick, ok := flowArmIndex(a.Choice, len(req.Options))
	strength := a.Strength()
	if !ok || strength < f.rt.deciderThreshold(decider.SiteFlowJudge) {
		rec.Outcome, rec.Strength = "unsure", strength
		f.rt.logDecision(rec)
		return -1, strength, nil
	}
	rec.Outcome, rec.Strength, rec.Applied = a.Choice, strength, true
	f.rt.logDecision(rec)
	return pick, strength, nil
}

// JudgeCondition implements orchestration.JudgeRunner.
func (f flowRunner) JudgeCondition(ctx context.Context, req orchestration.JudgeRequest) (bool, float64, error) {
	instructions := "Does the condition below hold for this output?\nCondition: " + req.Condition
	if q := strings.TrimSpace(req.Question); q != "" {
		instructions = q + "\nCondition: " + req.Condition
	}
	resp, err := f.rt.decide(ctx, decider.SiteFlowJudge, f.judgeCaller(ctx, req.AgentID), decider.Request{
		State: truncateRunes(req.Value, flowJudgeValueRunes),
		Questions: map[string]decider.Question{flowConditionKey: decider.Noul(instructions,
			req.Condition, "The condition does not hold yet.")},
	})
	rec := decider.NewRecord(decider.SiteFlowJudge, decider.ModeOn, resp, err)
	rec.Ref = flowRunIDFromContext(ctx)
	if err != nil {
		if !decisionOff(err) {
			f.rt.logDecision(rec)
		}
		return false, 0, err
	}
	a := resp.Answers[flowConditionKey]
	holds := a.Yes(f.rt.deciderThreshold(decider.SiteFlowJudge))
	rec.Outcome, rec.Strength, rec.Applied = strconv.FormatBool(holds), a.Probability, true
	f.rt.logDecision(rec)
	return holds, a.Probability, nil
}

// judgeCaller resolves the agent a judgement is billed to: the agent whose
// output is judged. An unknown or missing agent bills nobody (the decider
// ledger still records the cost).
func (f flowRunner) judgeCaller(ctx context.Context, agentID string) db.Agent {
	if agentID == "" {
		return db.Agent{}
	}
	a, err := f.rt.db.GetAgent(ctx, agentID)
	if err != nil {
		return db.Agent{}
	}
	return a
}

// flowArmOption is the option key of the i-th arm ("arm1", "arm2", …).
func flowArmOption(i int) string {
	return fmt.Sprintf("arm%d", i+1)
}

// flowArmIndex maps an option key back to its arm index.
func flowArmIndex(key string, n int) (int, bool) {
	i, err := strconv.Atoi(strings.TrimPrefix(key, "arm"))
	if err != nil || !strings.HasPrefix(key, "arm") || i < 1 || i > n {
		return -1, false
	}
	return i - 1, true
}
