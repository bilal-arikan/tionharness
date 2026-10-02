package decider

import (
	"context"
	"math"
	"strconv"
)

// QuestionType names one of the typed question shapes a decision model answers.
type QuestionType string

const (
	// QuestionNoul is a yes/no question answered with the probability that the
	// statement holds ("noul" is the System One API's name for it).
	QuestionNoul QuestionType = "noul"
	// QuestionChoice picks exactly one option out of a labelled set.
	QuestionChoice QuestionType = "choice"
	// QuestionScore rates the state against an ordered list of levels.
	QuestionScore QuestionType = "score"
)

// Question is one typed question. Which criteria fields apply depends on Type:
//
//   - noul:   True/False describe when the answer is yes/no. Optional, but
//     the System One API rejects one side without the other, so Normalize
//     fills a missing side.
//   - choice: Options maps an option key to its description (at least two).
//   - score:  Levels lists the scale, lowest first (at least two).
type Question struct {
	Type         QuestionType      `json:"type"`
	Instructions string            `json:"instructions,omitempty"`
	True         string            `json:"true,omitempty"`
	False        string            `json:"false,omitempty"`
	Options      map[string]string `json:"options,omitempty"`
	Levels       []string          `json:"levels,omitempty"`
}

// Request asks a decision model several questions about one state at once. The
// model answers all of them in a single pass, so adding a question costs only
// its own input tokens, not another round-trip.
type Request struct {
	// State is what the questions are about: a string, or any JSON-marshalable
	// value (map, struct, slice).
	State any `json:"state"`
	// Questions maps a caller-chosen key to its question; answers come back
	// under the same keys.
	Questions map[string]Question `json:"questions"`
	// Model overrides the decision model's configured service model id for
	// this one request ("" = the model instance's own id).
	Model string `json:"model,omitempty"`
}

// Answer is the model's answer to one question.
type Answer struct {
	Type QuestionType `json:"type"`
	// Probability is P(yes) for a noul question.
	Probability float64 `json:"probability,omitempty"`
	// Choice is the picked option key for a choice question.
	Choice string `json:"choice,omitempty"`
	// Score is the level index for a score question. It may be fractional (an
	// expectation over the levels); Level rounds it.
	Score float64 `json:"score,omitempty"`
	// Probabilities is the distribution over options (choice) or over level
	// indexes rendered as "0", "1", … (score). Rounded by the service, so it
	// may not sum to exactly 1.
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	// Confidence is the model's own confidence in a choice or score (0 when
	// the backend did not report one).
	Confidence float64 `json:"confidence,omitempty"`
}

// Yes reports whether a noul answer clears threshold. Always false for other
// question types, so a mismatched answer can never read as consent.
func (a Answer) Yes(threshold float64) bool {
	return a.Type == QuestionNoul && a.Probability >= threshold
}

// Level returns a score answer's level index rounded to the nearest integer.
func (a Answer) Level() int {
	return int(math.Round(a.Score))
}

// Strength is how firmly the model holds its answer, on a 0..1 scale: the
// probability of the more likely side for a noul question, the picked option's
// probability for a choice (its confidence when no distribution was reported)
// and the confidence for a score.
func (a Answer) Strength() float64 {
	switch a.Type {
	case QuestionNoul:
		return math.Max(a.Probability, 1-a.Probability)
	case QuestionChoice:
		if p, ok := a.Probabilities[a.Choice]; ok {
			return p
		}
		return a.Confidence
	case QuestionScore:
		if a.Confidence > 0 {
			return a.Confidence
		}
		return a.Probabilities[strconv.Itoa(a.Level())]
	}
	return 0
}

// Usage is what one decision call cost.
type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	// CostUSD is the cost the service reported, 0 when it reported none.
	CostUSD float64 `json:"costUsd"`
	// CostSource distinguishes a measured zero from missing or estimated pricing.
	CostSource string `json:"costSource,omitempty"`
}

// Response is a decision model's answers plus accounting.
type Response struct {
	// DebugID correlates model attempts, transport retries and the applied outcome.
	DebugID string `json:"debugId,omitempty"`
	// ID is the service's generation id, when it returns one.
	ID string `json:"id,omitempty"`
	// Backend is the backend that served the call.
	Backend string `json:"backend"`
	// Instance is the decision model instance (settings entry) that answered;
	// set by the Hub.
	Instance string `json:"instance,omitempty"`
	// Model is the service model id that was REQUESTED.
	Model string `json:"model"`
	// ServedModel is the concrete model/snapshot the service says answered.
	ServedModel string `json:"servedModel,omitempty"`
	// BillingProvider and BillingModel are the price-table provider and model
	// id the call is billed under ("openrouter" + "typesafe/jev-1.13", "local"
	// for a server on the user's own network). BillingModel "" = Model. The
	// served snapshot is never used for billing: no price table lists it.
	BillingProvider string `json:"billingProvider,omitempty"`
	BillingModel    string `json:"billingModel,omitempty"`
	// Fallback is set when the authority's primary model could not answer and
	// its fallback model did.
	Fallback bool `json:"fallback,omitempty"`
	// Warnings say how far the answers can be trusted when the backend had to
	// degrade (e.g. a local model returned no token probabilities, so every
	// answer is a hard 0/1).
	Warnings []string `json:"warnings,omitempty"`
	// Answers holds one answer per requested question key.
	Answers   map[string]Answer `json:"answers"`
	Usage     Usage             `json:"usage"`
	LatencyMs int64             `json:"latencyMs"`
}

// BilledModel is the model id the call is billed under.
func (r *Response) BilledModel() string {
	if r.BillingModel != "" {
		return r.BillingModel
	}
	return r.Model
}

// Decider answers typed questions about a state. Backends build one per
// endpoint + model; the Hub wraps them.
type Decider interface {
	Decide(ctx context.Context, req Request) (*Response, error)
}
