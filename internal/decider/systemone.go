package decider

import (
	"context"
	"fmt"
	"strings"
)

// System One backend: TypeSafe's native decision API (POST …/v1/systemone) and
// every server that speaks it — TypeSafe itself, OpenRouter's /api/v1/systemone,
// and the open OpenJev servers that run a decision model on the user's own GPU
// (they copy the wire format so TypeSafe's SDKs work against them unchanged).
// One backend therefore covers the hosted model and its local stand-ins; only
// the base URL, the key and the model id differ.
const (
	SystemOneBackendID = "systemone"

	// JevNativeModel is TypeSafe's floating alias; OpenJev servers accept it too.
	JevNativeModel = "jev-latest"
	// JevPinnedModel is the pinned snapshot id at TypeSafe and (mapped onto the
	// typesafe/ namespace) at OpenRouter.
	JevPinnedModel = "jev-1.13"
	// OpenJevModel is the model id the OpenJev reference server answers to.
	OpenJevModel = "openjev-latest"

	typeSafeDefaultBase = "https://api.typesafe.ai/v1"
	openJevDefaultBase  = "http://127.0.0.1:8080/v1"
	systemOneErrPrefix  = "systemone"
)

func init() { Register(systemOneBackend{}) }

type systemOneBackend struct{}

func (systemOneBackend) Manifest() Manifest {
	return Manifest{
		ID:             SystemOneBackendID,
		Label:          "System One API (TypeSafe · OpenJev)",
		Description:    "TypeSafe's native decision API and every server that speaks it: TypeSafe itself, OpenRouter's System One endpoint, and open OpenJev servers running a decision model on your own hardware.",
		ProviderKinds:  []string{"openrouter", "openai-compat"},
		DefaultBaseURL: typeSafeDefaultBase,
		// TypeSafe needs a key, a local OpenJev server usually does not; the
		// TypeSafe preset is the one that asks for it.
		KeyRequired: false,
		Models: []Model{
			{ID: JevNativeModel, Label: "Jev (latest)", Description: "TypeSafe's floating alias; OpenJev servers answer to it too.", InputPerMTok: jevInputPerMTok},
			{ID: JevPinnedModel, Label: "Jev 1.13", Description: "Pinned snapshot at TypeSafe and OpenRouter.", InputPerMTok: jevInputPerMTok},
			{ID: OpenJevModel, Label: "OpenJev (local)", Description: "The open OpenJev server's own model id. Free: it runs on your hardware."},
		},
		DefaultModel: JevNativeModel,
		// Conservative: OpenJev serves 16k-token prompts. The hosted presets
		// raise it to TypeSafe's 32k per question.
		ContextTokens:    16000,
		DefaultTimeoutMs: int(DefaultTimeout.Milliseconds()),
		// OpenJev scores at most 52 options in one pass; TypeSafe accepts two to
		// ten score levels.
		Limits:        Limits{MaxOptions: 52, MaxLevels: 10},
		ModelPrefixes: []string{"jev-", "openjev"},
		DecisionOnly:  true,
		Calibrated:    true,
		Presets: []Preset{
			{
				ID:            "jev-typesafe",
				Label:         "Jev · TypeSafe",
				Description:   "TypeSafe's own API with a TypeSafe API key.",
				Credentials:   CredentialsOwn,
				BaseURL:       typeSafeDefaultBase,
				Model:         JevNativeModel,
				ContextTokens: 32000,
			},
			{
				ID:            "jev-openrouter-systemone",
				Label:         "Jev · OpenRouter (System One)",
				Description:   "OpenRouter's System One endpoint through the key of an OpenRouter provider you already have.",
				Credentials:   CredentialsProvider,
				Model:         JevPinnedModel,
				ContextTokens: 32000,
			},
			{
				ID:            "openjev-local",
				Label:         "OpenJev · this machine",
				Description:   "An OpenJev server on this computer (docker compose up, default port 8080). Free and private; needs a GPU.",
				Credentials:   CredentialsOwn,
				BaseURL:       openJevDefaultBase,
				Model:         OpenJevModel,
				ContextTokens: 16000,
				TimeoutMs:     10000,
			},
		},
	}
}

// Accepts lends OpenRouter credentials only: OpenRouter serves the System One
// path under its own API base, and no other provider kind is known to.
func (systemOneBackend) Accepts(kind, baseURL string) bool {
	return acceptsOpenRouter(kind, baseURL)
}

func (b systemOneBackend) New(ep Endpoint, opts ClientOptions) (Decider, error) {
	if ep.Kind != "" && !b.Accepts(ep.Kind, ep.BaseURL) {
		return nil, fmt.Errorf("provider instance %q (%s) cannot reach a System One API", ep.InstanceID, ep.Kind)
	}
	root, err := endpointBase(ep)
	if err != nil {
		return nil, err
	}
	endpoint, err := SystemOneURL(root)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = JevNativeModel
	}
	return &systemOneClient{
		call: systemOneCall{
			client:    opts.HTTPClient,
			prefix:    systemOneErrPrefix,
			url:       endpoint,
			authorize: ep.Authorize,
			timeout:   timeoutOrDefault(opts.Timeout),
			model:     model,
		},
		billing: billingProviderFor(endpoint, ep.Kind),
	}, nil
}

// SystemOneURL derives the request URL from a base URL: ".../v1" gets
// "/systemone" appended, a full ".../systemone" URL is kept, and an SDK-style
// base without a version ("https://api.typesafe.ai") gets "/v1/systemone" —
// the rule TypeSafe's SDKs apply. "" = TypeSafe's API.
func SystemOneURL(base string) (string, error) {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if b == "" {
		b = typeSafeDefaultBase
	}
	if !strings.HasPrefix(b, "http://") && !strings.HasPrefix(b, "https://") {
		return "", fmt.Errorf("base URL %q must start with http:// or https://", base)
	}
	switch {
	case strings.HasSuffix(b, "/systemone"):
		return b, nil
	case strings.HasSuffix(b, "/v1"):
		return b + "/systemone", nil
	default:
		return b + "/v1/systemone", nil
	}
}

type systemOneClient struct {
	call    systemOneCall
	billing string
}

func (c *systemOneClient) Decide(ctx context.Context, req Request) (*Response, error) {
	resp, err := c.call.decide(ctx, req)
	if resp == nil {
		return resp, err
	}
	resp.Backend = SystemOneBackendID
	resp.BillingProvider = c.billing
	// OpenRouter maps TypeSafe's bare ids onto its typesafe/ namespace, and
	// that is how its price table lists them.
	if c.billing == billingOpenRouter && !strings.Contains(resp.Model, "/") {
		resp.BillingModel = "typesafe/" + resp.Model
	}
	return resp, err
}
