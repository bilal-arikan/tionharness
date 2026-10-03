package decider

import "testing"

func TestPick(t *testing.T) {
	resp := &Response{Answers: map[string]Answer{
		"arm": {Type: QuestionChoice, Choice: "b", Probabilities: map[string]float64{"a": 0.35, "b": 0.65}},
		"yes": {Type: QuestionNoul, Probability: 0.9},
	}}
	if c, s, ok := Pick(resp, "arm", 0.6); c != "b" || s != 0.65 || !ok {
		t.Errorf("pick = %q %v %v", c, s, ok)
	}
	if c, _, ok := Pick(resp, "arm", 0.7); c != "b" || ok {
		t.Errorf("below threshold = %q %v; the lean must still be reported", c, ok)
	}
	if _, _, ok := Pick(resp, "yes", 0); ok {
		t.Error("a noul answer read as a pick")
	}
	if _, _, ok := Pick(nil, "arm", 0); ok {
		t.Error("nil response picked")
	}
}

func TestVerdict(t *testing.T) {
	one := &Response{Answers: map[string]Answer{"q": {Type: QuestionNoul, Probability: 0.75}}}
	if v, s := Verdict(one, 0.7); v != "yes" || s != 0.75 {
		t.Errorf("verdict = %q %v", v, s)
	}
	if v, _ := Verdict(one, 0.8); v != "no" {
		t.Errorf("verdict at a higher threshold = %q", v)
	}
	many := &Response{Answers: map[string]Answer{
		"risk": {Type: QuestionScore, Score: 1.6, Confidence: 0.6},
		"pick": {Type: QuestionChoice, Choice: "x", Confidence: 0.9},
	}}
	if v, s := Verdict(many, 0.5); v != "pick=x,risk=L2" || s != 0.6 {
		t.Errorf("verdict = %q %v", v, s)
	}
	if v, _ := Verdict(nil, 0.5); v != "" {
		t.Error("nil verdict not empty")
	}
}
