package decider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// OpenRouter Decisions backend: OpenRouter's alpha Decisions API, which serves
// TypeSafe's Jev ("System One") models. The chat endpoint rejects these models
// outright ("… is a decisions model and cannot be used with the chat/completions
// endpoint"), and they are absent from OpenRouter's public /models catalog, so
// the ids are listed here rather than discovered.
const (
	OpenRouterBackendID = "openrouter"

	// JevModel is pinned by default: the "~…-latest" alias floats to whatever
	// snapshot TypeSafe ships next, which makes shadow-mode agreement numbers
	// incomparable across the switch.
	JevModel       = "typesafe/jev-1.13"
	JevLatestModel = "~typesafe/jev-latest"

	openRouterDefaultBase = "https://openrouter.ai/api/v1"
	openRouterHost        = "openrouter.ai"
	openRouterErrPrefix   = "openrouter-decisions"
	// Jev's input price on OpenRouter; output tokens are free.
	jevInputPerMTok = 0.042
)

func init() { Register(openRouterBackend{}) }

type openRouterBackend struct{}

func (openRouterBackend) Manifest() Manifest {
	return Manifest{
		ID:              OpenRouterBackendID,
		Label:           "OpenRouter Decisions",
		Description:     "OpenRouter's Decisions API (alpha). Serves TypeSafe's Jev: yes/no, pick-one and score answers with calibrated probabilities in well under a second.",
		ProviderKinds:   []string{"openrouter", "openai-compat"},
		BillingProvider: "openrouter",
		Models: []Model{
			{ID: JevModel, Label: "Jev 1.13", Description: "Pinned snapshot; the default.", InputPerMTok: jevInputPerMTok},
			{ID: JevLatestModel, Label: "Jev (latest)", Description: "Alias that follows TypeSafe's newest Jev snapshot.", InputPerMTok: jevInputPerMTok},
		},
		DefaultModel:  JevModel,
		ContextTokens: 32000,
		ModelPrefixes: []string{"typesafe/", "~typesafe/"},
	}
}

// Accepts takes any openrouter instance, and an openai-compat instance only
// when its base URL is on openrouter.ai (the Marketplace "openrouter" pack
// installs as openai-compat).
func (openRouterBackend) Accepts(kind, baseURL string) bool {
	switch kind {
	case "openrouter":
		return true
	case "openai-compat":
		u, err := url.Parse(strings.TrimSpace(baseURL))
		return err == nil && strings.EqualFold(u.Hostname(), openRouterHost)
	}
	return false
}

func (b openRouterBackend) New(ep Endpoint, opts ClientOptions) (Decider, error) {
	if !b.Accepts(ep.Kind, ep.BaseURL) {
		return nil, fmt.Errorf("provider instance %q (%s) cannot reach OpenRouter", ep.InstanceID, ep.Kind)
	}
	endpoint, err := DecisionsURL(ep.BaseURL)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = JevModel
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &openRouterClient{url: endpoint, authorize: ep.Authorize, model: model, timeout: timeout, client: opts.HTTPClient}, nil
}

// DecisionsURL derives the Decisions endpoint from an OpenRouter instance's base
// URL. Chat lives at ".../api/v1", decisions at ".../api/alpha/decisions". A base
// that does not end in "/v1" is refused rather than guessed: appending the path
// to an arbitrary proxy URL would send the key somewhere unintended.
func DecisionsURL(base string) (string, error) {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if b == "" {
		b = openRouterDefaultBase
	}
	if !strings.HasSuffix(b, "/v1") {
		return "", fmt.Errorf("cannot derive the Decisions endpoint from base URL %q: expected it to end in /v1", base)
	}
	return strings.TrimSuffix(b, "/v1") + "/alpha/decisions", nil
}

type openRouterClient struct {
	url       string
	authorize func(http.Header)
	model     string
	timeout   time.Duration
	client    *http.Client // nil = the shared client
}

// Wire shapes of the Decisions API (identical to TypeSafe's own API apart from
// the model slug and the extra id/provider/cost fields OpenRouter adds).
type orQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

type orRequest struct {
	Model     string                `json:"model"`
	State     any                   `json:"state"`
	Questions map[string]orQuestion `json:"questions"`
}

type orAnswer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul"`
	Choice        *string            `json:"choice"`
	Score         *float64           `json:"score"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    *float64           `json:"confidence"`
}

type orResponse struct {
	ID       string              `json:"id"`
	Model    string              `json:"model"`
	Provider string              `json:"provider"`
	Answers  map[string]orAnswer `json:"answers"`
	Usage    struct {
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
		Cost         float64 `json:"cost"`
	} `json:"usage"`
}

func (c *openRouterClient) Decide(ctx context.Context, req Request) (*Response, error) {
	req = req.Normalized()
	if err := req.Validate(); err != nil {
		return nil, err
	}
	model := c.model
	if m := strings.TrimSpace(req.Model); m != "" {
		model = m
	}
	body := orRequest{Model: model, State: req.State, Questions: make(map[string]orQuestion, len(req.Questions))}
	for k, q := range req.Questions {
		body.Questions[k] = toWireQuestion(q)
	}
	start := time.Now()
	var out orResponse
	if err := postJSON(ctx, c.client, openRouterErrPrefix, c.url, c.authorize, c.timeout, body, &out); err != nil {
		return nil, err
	}
	resp := &Response{
		ID:              out.ID,
		Backend:         OpenRouterBackendID,
		Model:           model,
		ServedModel:     out.Model,
		BillingProvider: "openrouter",
		Answers:         make(map[string]Answer, len(out.Answers)),
		Usage:           Usage{InputTokens: out.Usage.InputTokens, OutputTokens: out.Usage.OutputTokens, CostUSD: out.Usage.Cost},
		LatencyMs:       time.Since(start).Milliseconds(),
	}
	for k, q := range req.Questions {
		wa, ok := out.Answers[k]
		if !ok {
			return nil, fmt.Errorf("%s: response has no answer for question %q", openRouterErrPrefix, k)
		}
		a, err := fromWireAnswer(q.Type, wa)
		if err != nil {
			return nil, fmt.Errorf("%s: question %q: %w", openRouterErrPrefix, k, err)
		}
		resp.Answers[k] = a
	}
	return resp, nil
}

func toWireQuestion(q Question) orQuestion {
	w := orQuestion{Type: string(q.Type), Instructions: q.Instructions}
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
func fromWireAnswer(want QuestionType, w orAnswer) (Answer, error) {
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
