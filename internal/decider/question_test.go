package decider

import (
	"strings"
	"testing"
)

func TestRequestValidate(t *testing.T) {
	ok := Request{State: "x", Questions: map[string]Question{"q": Noul("?", "", "")}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	cases := []struct {
		name string
		req  Request
		want string
	}{
		{"no state", Request{Questions: ok.Questions}, "no state"},
		{"blank state", Request{State: "  ", Questions: ok.Questions}, "empty state"},
		{"no questions", Request{State: "x"}, "no questions"},
		{"bad key", Request{State: "x", Questions: map[string]Question{"a b": Noul("?", "", "")}}, "invalid character"},
		{"one option", Request{State: "x", Questions: map[string]Question{"q": Choice("?", map[string]string{"a": "A"})}}, "at least two options"},
		{"one level", Request{State: "x", Questions: map[string]Question{"q": Score("?", "low")}}, "at least two levels"},
		{"empty level", Request{State: "x", Questions: map[string]Question{"q": Score("?", "low", " ")}}, "level 1 is empty"},
		{"unknown type", Request{State: "x", Questions: map[string]Question{"q": {Type: "rank"}}}, "unknown question type"},
	}
	for _, c := range cases {
		err := c.req.Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", c.name, err, c.want)
		}
	}
}

// The Decisions API rejects a noul question that describes only one side, so a
// half-specified pair must be completed before sending.
func TestNormalizedCompletesHalfNoulCriteria(t *testing.T) {
	req := Request{State: "x", Questions: map[string]Question{
		"onlyTrue":  Noul("?", "yes when X", ""),
		"onlyFalse": Noul("?", "", "no when Y"),
		"neither":   Noul("?", "", ""),
	}}
	n := req.Normalized()
	if q := n.Questions["onlyTrue"]; q.True != "yes when X" || q.False == "" {
		t.Errorf("onlyTrue = %+v, want a filled false side", q)
	}
	if q := n.Questions["onlyFalse"]; q.False != "no when Y" || q.True == "" {
		t.Errorf("onlyFalse = %+v, want a filled true side", q)
	}
	if q := n.Questions["neither"]; q.True != "" || q.False != "" {
		t.Errorf("neither = %+v, want both sides left empty", q)
	}
	if req.Questions["onlyTrue"].False != "" {
		t.Error("Normalized mutated the original request")
	}
}

func TestAnswerHelpers(t *testing.T) {
	yes := Answer{Type: QuestionNoul, Probability: 0.94}
	if !yes.Yes(0.9) || yes.Yes(0.95) {
		t.Errorf("Yes thresholds wrong for p=0.94")
	}
	if got := (Answer{Type: QuestionNoul, Probability: 0.1}).Strength(); got != 0.9 {
		t.Errorf("noul strength = %v, want 0.9", got)
	}
	choice := Answer{Type: QuestionChoice, Choice: "b", Probabilities: map[string]float64{"a": 0.23, "b": 0.77}, Confidence: 0.66}
	if choice.Strength() != 0.77 {
		t.Errorf("choice strength = %v, want the picked option's probability", choice.Strength())
	}
	if (Answer{Type: QuestionChoice, Choice: "b", Confidence: 0.66}).Strength() != 0.66 {
		t.Error("choice without distribution should fall back to confidence")
	}
	score := Answer{Type: QuestionScore, Score: 1.6, Confidence: 0.9}
	if score.Level() != 2 || score.Strength() != 0.9 {
		t.Errorf("score level/strength = %d/%v", score.Level(), score.Strength())
	}
	// A choice answer must never read as a "yes".
	if choice.Yes(0) {
		t.Error("a choice answer passed Yes()")
	}
}
