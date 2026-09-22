package decider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The System One wire format: TypeSafe's own API (POST /v1/systemone), which
// OpenRouter serves both at /api/v1/systemone and (alpha) at
// /api/alpha/decisions, and which the open OpenJev servers copy so TypeSafe's
// SDKs work against them unchanged. OpenRouter adds id, provider and
// usage.cost; local servers omit what they cannot fill. Decoding is therefore
// tolerant: every answer field is optional, and a score's distribution may
// arrive keyed by level ({"0": p, …}) or as a plain array.

type soQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

type soRequest struct {
	Model     string                `json:"model"`
	State     any                   `json:"state"`
	Questions map[string]soQuestion `json:"questions"`
}

type soAnswer struct {
	Type          string     `json:"type"`
	Noul          *float64   `json:"noul"`
	Choice        *string    `json:"choice"`
	Score         *float64   `json:"score"`
	Probabilities flexProbs  `json:"probabilities"`
	Confidence    *float64   `json:"confidence"`
	Legend        flexLegend `json:"legend"`
}

type soResponse struct {
	ID       string              `json:"id"`
	Model    string              `json:"model"`
	Provider string              `json:"provider"`
	Answers  map[string]soAnswer `json:"answers"`
	Usage    struct {
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
		Cost         float64 `json:"cost"`
	} `json:"usage"`
}

// flexProbs decodes a probability distribution given either as an object keyed
// by option or level, or as an array indexed by level.
type flexProbs map[string]float64

func (p *flexProbs) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*p = nil
		return nil
	}
	if raw[0] == '[' {
		var list []float64
		if err := json.Unmarshal(raw, &list); err != nil {
			return err
		}
		out := make(flexProbs, len(list))
		for i, v := range list {
			out[strconv.Itoa(i)] = v
		}
		*p = out
		return nil
	}
	var m map[string]float64
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	*p = m
	return nil
}

// flexLegend accepts a score legend in any shape; the client never needs it
// (the levels were sent with the question), so it is decoded only to be
// tolerated.
type flexLegend struct{}

func (*flexLegend) UnmarshalJSON([]byte) error { return nil }

// systemOneCall is one System One request to url. prefix names the backend in
// errors ("openrouter-decisions HTTP 400: …").
type systemOneCall struct {
	client    *http.Client
	prefix    string
	url       string
	authorize func(http.Header)
	timeout   time.Duration
	model     string
}

// decide sends req and maps the answers. The caller fills the Response's
// backend and billing fields.
func (c systemOneCall) decide(ctx context.Context, req Request) (*Response, error) {
	req = req.Normalized()
	if err := req.Validate(); err != nil {
		return nil, err
	}
	model := c.model
	if m := strings.TrimSpace(req.Model); m != "" {
		model = m
	}
	body := soRequest{Model: model, State: req.State, Questions: make(map[string]soQuestion, len(req.Questions))}
	for k, q := range req.Questions {
		body.Questions[k] = toWireQuestion(q)
	}
	start := time.Now()
	var out soResponse
	if err := postJSON(ctx, c.client, c.prefix, c.url, c.authorize, c.timeout, body, &out); err != nil {
		return nil, err
	}
	resp := &Response{
		ID:          out.ID,
		Model:       model,
		ServedModel: out.Model,
		Answers:     make(map[string]Answer, len(out.Answers)),
		Usage:       Usage{InputTokens: out.Usage.InputTokens, OutputTokens: out.Usage.OutputTokens, CostUSD: out.Usage.Cost},
		LatencyMs:   time.Since(start).Milliseconds(),
	}
	for k, q := range req.Questions {
		wa, ok := out.Answers[k]
		if !ok {
			return nil, fmt.Errorf("%s: response has no answer for question %q", c.prefix, k)
		}
		a, err := fromWireAnswer(q.Type, wa)
		if err != nil {
			return nil, fmt.Errorf("%s: question %q: %w", c.prefix, k, err)
		}
		resp.Answers[k] = a
	}
	return resp, nil
}

func toWireQuestion(q Question) soQuestion {
	w := soQuestion{Type: string(q.Type), Instructions: q.Instructions}
	switch q.Type {
	case QuestionNoul:
		if q.True != "" || q.False != "" {
			w.Criteria = map[string]string{"true": q.True, "false": q.False}
		}
	case QuestionChoice:
		w.Criteria = q.Options
	case QuestionScore:
		w.Criteria = q.Levels
	}
	return w
}

// fromWireAnswer maps one wire answer onto the neutral Answer, checking that it
// answers the question that was asked.
func fromWireAnswer(want QuestionType, w soAnswer) (Answer, error) {
	if w.Type != "" && w.Type != string(want) {
		return Answer{}, fmt.Errorf("answered as %q, asked as %q", w.Type, want)
	}
	a := Answer{Type: want, Probabilities: w.Probabilities}
	if w.Confidence != nil {
		a.Confidence = *w.Confidence
	}
	switch want {
	case QuestionNoul:
		switch {
		case w.Noul != nil:
			a.Probability = *w.Noul
		case w.Probabilities != nil:
			p, ok := w.Probabilities["true"]
			if !ok {
				return Answer{}, fmt.Errorf("noul answer carries no probability")
			}
			a.Probability = p
		default:
			return Answer{}, fmt.Errorf("noul answer carries no probability")
		}
		a.Probability = clamp01(a.Probability)
	case QuestionChoice:
		if w.Choice == nil || *w.Choice == "" {
			return Answer{}, fmt.Errorf("choice answer names no option")
		}
		a.Choice = *w.Choice
	case QuestionScore:
		switch {
		case w.Score != nil:
			a.Score = *w.Score
		case len(w.Probabilities) > 0:
			a.Score = expectedLevel(w.Probabilities)
		default:
			return Answer{}, fmt.Errorf("score answer carries no score")
		}
	}
	return a, nil
}

// expectedLevel is the probability-weighted mean level of a score distribution
// keyed "0", "1", … (entries with other keys are ignored).
func expectedLevel(p map[string]float64) float64 {
	var sum, weight float64
	for k, v := range p {
		i, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		sum += float64(i) * v
		weight += v
	}
	if weight == 0 {
		return 0
	}
	return sum / weight
}

func clamp01(v float64) float64 {
	return min(max(v, 0), 1)
}
