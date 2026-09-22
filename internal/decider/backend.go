package decider

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Endpoint is what a backend needs to reach its service through one provider
// instance: the instance's kind and configured base URL, plus a function that
// stamps the instance's credential onto a request. It carries an authorizer
// rather than the key itself so the secret never travels through this layer as
// a plain string (and so can never end up in a log line or an error message).
type Endpoint struct {
	InstanceID string
	Kind       string
	BaseURL    string
	Authorize  func(http.Header)
}

// ClientOptions tune one backend client.
type ClientOptions struct {
	// Model is the decision model to ask. "" = the backend's default model.
	Model string
	// Timeout bounds ONE attempt; a retry gets its own. 0 = DefaultTimeout.
	Timeout time.Duration
	// HTTPClient overrides the shared client (tests). nil = shared default.
	HTTPClient *http.Client
}

// Model describes one decision model a backend offers. Prices are per million
// tokens and mirror the provider price table the calls are billed against.
type Model struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`
	Description   string  `json:"description,omitempty"`
	InputPerMTok  float64 `json:"inputPerMTok"`
	OutputPerMTok float64 `json:"outputPerMTok"`
}

// Manifest describes a backend to the settings UI and to the model guard.
type Manifest struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	// ProviderKinds lists the provider-instance kinds whose credentials this
	// backend can use. Accepts makes the final call (an openai-compat instance
	// qualifies only when it points at the right host).
	ProviderKinds []string `json:"providerKinds"`
	// BillingProvider is the price-table provider its calls are billed under.
	BillingProvider string  `json:"billingProvider"`
	Models          []Model `json:"models"`
	DefaultModel    string  `json:"defaultModel"`
	// ContextTokens bounds state + questions together, in tokens.
	ContextTokens int `json:"contextTokens"`
	// ModelPrefixes mark model ids that belong to this backend's decision-only
	// family (IsDecisionModel), so a custom id such as "typesafe/jev-2" is
	// recognised before any manifest lists it.
	ModelPrefixes []string `json:"modelPrefixes,omitempty"`
}

// Backend is one decision-model transport: a family of decision models behind
// one API. Backends self-register from init(), like provider kinds, so adding
// an alternative to the first backend touches no caller.
type Backend interface {
	Manifest() Manifest
	// Accepts reports whether an instance of this kind and base URL can reach
	// the backend's service.
	Accepts(kind, baseURL string) bool
	// New builds a client for one endpoint and model.
	New(ep Endpoint, opts ClientOptions) (Decider, error)
}

// DefaultTimeout bounds one decision attempt. Decision models answer in well
// under a second; three seconds leaves room for a slow network while keeping a
// hung call (seen on the Decisions endpoint) from stalling the caller.
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
