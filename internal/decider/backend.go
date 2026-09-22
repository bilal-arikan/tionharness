package decider

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Endpoint is a resolved connection to a decision service: the base URL a
// backend derives its request URL from, plus a function that stamps the
// credential onto a request. It carries an authorizer rather than the key so
// the secret never travels through this layer as a plain string (and so can
// never end up in a log line or an error message).
type Endpoint struct {
	// InstanceID and Kind name the provider instance whose credentials were
	// borrowed; both are "" when the decision model carries its own.
	InstanceID string
	Kind       string
	// BaseURL is the configured base URL ("" = the backend's default).
	BaseURL string
	// Authorize stamps the credential; nil for a server that needs none.
	Authorize func(http.Header)
}

// ClientOptions tune one backend client.
type ClientOptions struct {
	// Model is the service model id to ask. "" = the backend's default model.
	Model string
	// Timeout bounds ONE attempt; a retry gets its own. 0 = DefaultTimeout.
	Timeout time.Duration
	// Config carries the model instance's backend-specific field values
	// (Manifest.Fields, non-secret ones only).
	Config map[string]string
	// HTTPClient overrides the shared client (tests). nil = shared default.
	HTTPClient *http.Client
}

// Model describes one decision model a backend suggests. Prices are per million
// tokens and mirror the provider price table the calls are billed against.
type Model struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`
	Description   string  `json:"description,omitempty"`
	InputPerMTok  float64 `json:"inputPerMTok"`
	OutputPerMTok float64 `json:"outputPerMTok"`
}

// Field is one backend-specific setting of a decision model, rendered by the
// settings form from the manifest alone (like a provider kind's FieldSpec).
type Field struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Type selects the control: "text" | "number" | "textarea" | "select".
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
	Default     string   `json:"default,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
}

// Preset is a ready-made decision model the settings page offers as a one-click
// starting point ("OpenJev on this machine", "Jev through OpenRouter").
type Preset struct {
	ID          string           `json:"id"`
	Label       string           `json:"label"`
	Description string           `json:"description,omitempty"`
	Credentials CredentialSource `json:"credentials"`
	BaseURL     string           `json:"baseUrl,omitempty"`
	Model       string           `json:"model"`
	// ContextTokens overrides the backend's context size for this preset.
	ContextTokens int               `json:"contextTokens,omitempty"`
	TimeoutMs     int               `json:"timeoutMs,omitempty"`
	Config        map[string]string `json:"config,omitempty"`
}

// Limits are what one backend accepts beyond the general request limits. 0 =
// no extra limit.
type Limits struct {
	MaxOptions int `json:"maxOptions,omitempty"`
	MaxLevels  int `json:"maxLevels,omitempty"`
}

// Manifest describes a backend to the settings UI and to the model guard.
type Manifest struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	// ProviderKinds lists the provider-instance kinds whose credentials a
	// decision model of this backend can borrow. Accepts makes the final call
	// (an openai-compat instance qualifies only when it points at the right host).
	ProviderKinds []string `json:"providerKinds"`
	// DefaultBaseURL is used by a model with its own credentials and no base URL.
	DefaultBaseURL string `json:"defaultBaseUrl"`
	// KeyRequired: a model with its own credentials must carry an API key (a
	// hosted API). False for servers that usually run without one.
	KeyRequired bool `json:"keyRequired"`
	// Fields are the backend-specific settings of a model.
	Fields       []Field `json:"fields,omitempty"`
	Models       []Model `json:"models"`
	DefaultModel string  `json:"defaultModel"`
	// ContextTokens bounds state + questions together, in tokens, unless a
	// model instance overrides it.
	ContextTokens int `json:"contextTokens"`
	// DefaultTimeoutMs is a new model's per-attempt timeout.
	DefaultTimeoutMs int    `json:"defaultTimeoutMs"`
	Limits           Limits `json:"limits"`
	// ModelPrefixes mark model ids that belong to a decision-only family
	// (IsDecisionModel), so a custom id such as "typesafe/jev-2" is recognised
	// before any manifest lists it.
	ModelPrefixes []string `json:"modelPrefixes,omitempty"`
	// DecisionOnly marks backends whose models answer nothing but typed
	// questions: the chat endpoints reject them, so the agent API refuses them
	// as chat models. False for an adapter that turns an ordinary chat model
	// into a decision model.
	DecisionOnly bool `json:"decisionOnly"`
	// Calibrated reports whether the service returns calibrated probabilities.
	// An adapter reading an ordinary model's token probabilities is not.
	Calibrated bool     `json:"calibrated"`
	Presets    []Preset `json:"presets,omitempty"`
}

// FieldByKey returns the backend-specific field with key.
func (m Manifest) FieldByKey(key string) (Field, bool) {
	for _, f := range m.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// Backend is one decision-model transport: a family of decision models behind
// one API. Backends self-register from init(), like provider kinds, so adding
// an alternative touches no caller.
type Backend interface {
	Manifest() Manifest
	// Accepts reports whether a provider instance of this kind and base URL
	// can lend its credentials to reach the backend's service.
	Accepts(kind, baseURL string) bool
	// New builds a client for one endpoint and model.
	New(ep Endpoint, opts ClientOptions) (Decider, error)
}

// DefaultTimeout bounds one decision attempt. Hosted decision models answer in
// well under a second; three seconds leaves room for a slow network while
// keeping a hung call from stalling the caller.
const DefaultTimeout = 3 * time.Second

var (
	backendsMu sync.RWMutex
	backends   = map[string]Backend{}
)

// Register adds a backend. It panics on an empty or duplicate id: both are
// programming errors in an init() function, not runtime conditions.
func Register(b Backend) {
	m := b.Manifest()
	if m.ID == "" {
		panic("decider: backend with empty id")
	}
	backendsMu.Lock()
	defer backendsMu.Unlock()
	if _, dup := backends[m.ID]; dup {
		panic(fmt.Sprintf("decider: backend %q registered twice", m.ID))
	}
	backends[m.ID] = b
}

// Lookup returns the backend registered under id.
func Lookup(id string) (Backend, bool) {
	backendsMu.RLock()
	defer backendsMu.RUnlock()
	b, ok := backends[id]
	return b, ok
}

// Manifests lists every registered backend, sorted by id.
func Manifests() []Manifest {
	backendsMu.RLock()
	out := make([]Manifest, 0, len(backends))
	for _, b := range backends {
		out = append(out, b.Manifest())
	}
	backendsMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// IsDecisionModel reports whether model is a decision-only model of any
// registered backend. The agent API uses it to refuse such a model as a chat
// model: the chat endpoint rejects it and the agent would fail on every turn.
func IsDecisionModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	for _, m := range Manifests() {
		if !m.DecisionOnly {
			continue
		}
		for _, dm := range m.Models {
			if strings.EqualFold(dm.ID, model) {
				return true
			}
		}
		for _, p := range m.ModelPrefixes {
			if strings.HasPrefix(model, strings.ToLower(p)) {
				return true
			}
		}
	}
	return false
}

// timeoutOrDefault returns t, or DefaultTimeout when unset.
func timeoutOrDefault(t time.Duration) time.Duration {
	if t <= 0 {
		return DefaultTimeout
	}
	return t
}
