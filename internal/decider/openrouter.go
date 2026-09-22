package decider

import (
	"context"
	"fmt"
	"net/url"
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
		ProviderKinds:  []string{"openrouter", "openai-compat"},
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
			Description: "TypeSafe Jev through the key of an OpenRouter provider you already have.",
			Credentials: CredentialsProvider,
			Model:       JevModel,
		}},
	}
}

// Accepts takes any openrouter instance, and an openai-compat instance only
// when its base URL is on openrouter.ai (the Marketplace "openrouter" pack
// installs as openai-compat).
func (openRouterBackend) Accepts(kind, baseURL string) bool {
	return acceptsOpenRouter(kind, baseURL)
}

func acceptsOpenRouter(kind, baseURL string) bool {
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
	if ep.Kind != "" && !b.Accepts(ep.Kind, ep.BaseURL) {
		return nil, fmt.Errorf("provider instance %q (%s) cannot reach OpenRouter", ep.InstanceID, ep.Kind)
	}
	root, err := endpointBase(ep)
	if err != nil {
		return nil, err
	}
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
	if err != nil {
		return nil, err
	}
	resp.Backend = OpenRouterBackendID
	resp.BillingProvider = billingOpenRouter
	return resp, nil
}
