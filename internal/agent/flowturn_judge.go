package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/flow"
)

// Decision authority "flow-judge": a route node in judge mode asks the decision
// model which arm's label describes the last output. An error or an unsure
// answer falls back to the route's default arm (the engine's rule).
//
// Decision authority "flow-criteria": a route node in criteria mode asks one
// yes/no question per criterion in a single call; the engine takes the pass
// arm only when every criterion holds (flow.pickCriteriaEdge).

const (
	flowArmKey = "arm"
	// flowJudgeValueRunes bounds the judged output. The decider trims further
	// to its model's context; this keeps a runaway output from being redacted
	// and sent in full first.
	flowJudgeValueRunes = 24000
)

// judgeRouteArm returns the index of the option that describes value, or -1.
func (r *Runtime) judgeRouteArm(ctx context.Context, caller db.Agent, node flow.Node, value string, options []string) (int, error) {
	question := strings.TrimSpace(node.Question)
	if question == "" {
		question = "Which option best describes this output?"
	}
	opts := make(map[string]string, len(options))
	for i, o := range options {
		opts[flowArmOption(i)] = o
	}
	threshold := r.deciderThreshold(authFlowJudge)
	outcome := func(resp *decider.Response) (string, float64) {
		choice, strength, ok := decider.Pick(resp, flowArmKey, threshold)
		if _, known := flowArmIndex(choice, len(options)); !ok || !known {
			return "unsure", strength
		}
		return choice, strength
	}
	ref := flowRunIDFrom(ctx)
	resp, err := r.decide(ctx, authFlowJudge, caller, decider.Request{
		State:     truncateRunes(value, flowJudgeValueRunes),
		Questions: map[string]decider.Question{flowArmKey: decider.Choice(question, opts)},
	}, decider.WithRef(ref), decider.WithOutcome(outcome))
	rec := decider.NewRecord(authFlowJudge, decider.ModeOn, resp, err)
	rec.Ref = ref
	if err != nil {
		if !decisionOff(err) {
			r.logDecision(rec)
		}
		return -1, err
	}
	choice, strength, ok := decider.Pick(resp, flowArmKey, threshold)
	pick, known := flowArmIndex(choice, len(options))
	if !ok || !known {
		rec.Outcome, rec.Strength = "unsure", strength
		r.logDecision(rec)
		return -1, nil
	}
	rec.Outcome, rec.Strength, rec.Applied = choice, strength, true
	r.logDecision(rec)
	return pick, nil
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

// checkRouteCriteria reports, per criterion, whether value satisfies it. The
// whole list is one decision call (one noul question per criterion), so a
// gate with five criteria costs the same round trip as one.
func (r *Runtime) checkRouteCriteria(ctx context.Context, caller db.Agent, node flow.Node, value string, criteria []string) ([]bool, error) {
	if len(criteria) == 0 {
		return nil, errors.New("no criteria")
	}
	questions := make(map[string]decider.Question, len(criteria))
	for i, c := range criteria {
		questions[flowCriterionKey(i)] = decider.Noul(
			"Does the text satisfy this criterion? Criterion: "+strings.TrimSpace(c),
			"Yes: the text clearly satisfies the criterion.",
			"No: the text does not satisfy it, or it is unclear whether it does.")
	}
	threshold := r.deciderThreshold(authFlowCriteria)
	outcome := func(resp *decider.Response) (string, float64) {
		verdicts, strength := flowCriteriaVerdicts(resp, len(criteria), threshold)
		for _, ok := range verdicts {
			if !ok {
				return "fail", strength
			}
		}
		return "pass", strength
	}
	ref := flowRunIDFrom(ctx)
	resp, err := r.decide(ctx, authFlowCriteria, caller, decider.Request{
		State:     truncateRunes(value, flowJudgeValueRunes),
		Questions: questions,
	}, decider.WithRef(ref), decider.WithOutcome(outcome))
	rec := decider.NewRecord(authFlowCriteria, decider.ModeOn, resp, err)
	rec.Ref = ref
	if err != nil {
		if !decisionOff(err) {
			r.logDecision(rec)
		}
		return nil, err
	}
	verdicts, strength := flowCriteriaVerdicts(resp, len(criteria), threshold)
	rec.Outcome, rec.Strength = outcome(resp)
	rec.Strength, rec.Applied = strength, true
	r.logDecision(rec)
	return verdicts, nil
}

// flowCriterionKey is the question key of the i-th criterion ("c1", "c2", …).
func flowCriterionKey(i int) string {
	return fmt.Sprintf("c%d", i+1)
}

// flowCriteriaVerdicts reads the per-criterion answers; a missing answer is a
// failed criterion. The strength is the weakest answer's.
func flowCriteriaVerdicts(resp *decider.Response, n int, threshold float64) ([]bool, float64) {
	verdicts := make([]bool, n)
	strength := 1.0
	if resp == nil {
		return verdicts, 0
	}
	for i := range verdicts {
		a, ok := resp.Answers[flowCriterionKey(i)]
		if !ok {
			strength = 0
			continue
		}
		verdicts[i] = a.Yes(threshold)
		if s := a.Strength(); s < strength {
			strength = s
		}
	}
	return verdicts, strength
}
