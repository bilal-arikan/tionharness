package decider

import (
	"context"
	"fmt"
	"strings"
)

// OpenRouter Decisions backend: OpenRouter's alpha Decisions API, which serves
// TypeSafe's Jev ("System One") models in the System One wire format. The chat
// endpoint rejects these models outright ("… is a decisions model and cannot be
// used with the chat/completions endpoint"), and they are absent from
// OpenRouter's public /models catalog, so the ids are listed here rather than
// discovered. OpenRouter also serves the same models at /api/v1/systemone; the
// System One backend (systemone.go) reaches that path.
const (
	OpenRouterBackendID = "openrouter"

	// JevModel is pinned by default: the "~…-latest" alias floats to whatever
	// snapshot TypeSafe ships next, which makes shadow-mode agreement numbers
	// incomparable across the switch.
	JevModel       = "typesafe/jev-1.13"
	JevLatestModel = "~typesafe/jev-latest"

	openRouterDefaultBase = "https://openrouter.ai/api/v1"
	openRouterErrPrefix   = "openrouter-decisions"
	// Jev's input price on OpenRouter and at TypeSafe; output tokens are free.
	jevInputPerMTok = 0.042
)

func init() { Register(openRouterBackend{}) }

type openRouterBackend struct{}

func (openRouterBackend) Manifest() Manifest {
	return Manifest{
		ID:             OpenRouterBackendID,
		Label:          "OpenRouter Decisions",
		Description:    "OpenRouter's Decisions API (alpha). Serves TypeSafe's Jev: yes/no, pick-one and score answers with calibrated probabilities in well under a second.",
		DefaultBaseURL: openRouterDefaultBase,
		KeyRequired:    true,
		Models: []Model{
			{ID: JevModel, Label: "Jev 1.13", Description: "Pinned snapshot; the default.", InputPerMTok: jevInputPerMTok},
			{ID: JevLatestModel, Label: "Jev (latest)", Description: "Alias that follows TypeSafe's newest Jev snapshot.", InputPerMTok: jevInputPerMTok},
		},
		DefaultModel:     JevModel,
		ContextTokens:    32000,
		DefaultTimeoutMs: int(DefaultTimeout.Milliseconds()),
		Limits:           Limits{MaxOptions: 255, MaxLevels: 10},
		ModelPrefixes:    []string{"typesafe/", "~typesafe/"},
		DecisionOnly:     true,
		Calibrated:       true,
		Presets: []Preset{{
			ID:          "jev-openrouter",
			Label:       "Jev · OpenRouter",
			Description: "TypeSafe Jev through OpenRouter with your OpenRouter API key.",
			Credentials: CredentialsOwn,
			BaseURL:     openRouterDefaultBase,
			Model:       JevModel,
		}},
	}
}

func (b openRouterBackend) New(ep Endpoint, opts ClientOptions) (Decider, error) {
	root := endpointBase(ep)
	endpoint, err := DecisionsURL(root)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = JevModel
	}
	return &openRouterClient{call: systemOneCall{
		client:    opts.HTTPClient,
		prefix:    openRouterErrPrefix,
		url:       endpoint,
		authorize: ep.Authorize,
		timeout:   timeoutOrDefault(opts.Timeout),
		model:     model,
	}}, nil
}

// DecisionsURL derives the Decisions endpoint from an OpenRouter base URL. Chat
// lives at ".../api/v1", decisions at ".../api/alpha/decisions". A base that
// does not end in "/v1" is refused rather than guessed: appending the path to an
// arbitrary proxy URL would send the key somewhere unintended.
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
	call systemOneCall
}

// Decide bills every call to OpenRouter, whatever host the base URL names: the
// Decisions API exists only there, so a proxy in front of it still spends
// OpenRouter credit.
func (c *openRouterClient) Decide(ctx context.Context, req Request) (*Response, error) {
	resp, err := c.call.decide(ctx, req)
	if resp == nil {
		return resp, err
	}
	resp.Backend = OpenRouterBackendID
	resp.BillingProvider = billingOpenRouter
	return resp, err
}
