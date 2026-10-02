package decider

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecisionAnswerRejectsInvalidProviderValues(t *testing.T) {
	choice := Choice("pick", map[string]string{"a": "first", "b": "second"})
	score := Score("rate", "low", "high")
	for _, tc := range []struct {
		name string
		q    Question
		a    Answer
	}{
		{"above-one", Noul("yes?", "", ""), Answer{Type: QuestionNoul, Probability: 1.2}},
		{"negative", Noul("yes?", "", ""), Answer{Type: QuestionNoul, Probability: -.1}},
		{"nan", Noul("yes?", "", ""), Answer{Type: QuestionNoul, Probability: math.NaN()}},
		{"invented-option", choice, Answer{Type: QuestionChoice, Choice: "not-requested"}},
		{"bad-confidence", choice, Answer{Type: QuestionChoice, Choice: "a", Confidence: 1.1}},
		{"bad-distribution", choice, Answer{Type: QuestionChoice, Choice: "a", Probabilities: map[string]float64{"a": -.1}}},
		{"unknown-distribution-key", choice, Answer{Type: QuestionChoice, Choice: "a", Probabilities: map[string]float64{"c": .1}}},
		{"bad-score", score, Answer{Type: QuestionScore, Score: 2}},
		{"infinite-score", score, Answer{Type: QuestionScore, Score: math.Inf(1)}},
		{"unknown-level", score, Answer{Type: QuestionScore, Score: 0, Probabilities: map[string]float64{"2": .5}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if validateDecisionAnswer(tc.q, tc.a) == nil {
				t.Fatal("accepted invalid answer")
			}
		})
	}
}

func TestSystemOneDoesNotClampInvalidProbabilityIntoApproval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{"q":{"type":"noul","noul":1.2}}}`))
	}))
	defer srv.Close()
	c := systemOneCall{url: srv.URL, prefix: "test", model: OpenJevModel}
	resp, err := c.decide(context.Background(), yesNo())
	if resp == nil || len(resp.Answers) != 0 || !errors.Is(err, ErrInvalidResponse) || errorClass(err) != "invalid_response" {
		t.Fatalf("answer = %+v %v", resp, err)
	}
}
