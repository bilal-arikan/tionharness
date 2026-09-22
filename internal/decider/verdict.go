package decider

import (
	"strconv"
	"strings"
)

// Verdict renders a response as one comparable verdict, so two models' answers
// to the same request can be checked for agreement without knowing what the
// questions mean: per question "yes"/"no" at threshold, the chosen option, or
// "L<level>"; several questions are joined as "key=verdict" in key order. The
// strength is the weakest answer's.
func Verdict(resp *Response, threshold float64) (string, float64) {
	if resp == nil || len(resp.Answers) == 0 {
		return "", 0
	}
	keys := sortedKeys(resp.Answers)
	parts := make([]string, 0, len(keys))
	strength := 1.0
	for _, k := range keys {
		a := resp.Answers[k]
		v := answerVerdict(a, threshold)
		if len(keys) > 1 {
			v = k + "=" + v
		}
		parts = append(parts, v)
		strength = min(strength, a.Strength())
	}
	return strings.Join(parts, ","), strength
}

func answerVerdict(a Answer, threshold float64) string {
	switch a.Type {
	case QuestionNoul:
		if a.Yes(threshold) {
			return "yes"
		}
		return "no"
	case QuestionChoice:
		return a.Choice
	case QuestionScore:
		return "L" + strconv.Itoa(a.Level())
	}
	return "?"
}
