package decider

import (
	"fmt"
	"maps"
	"strings"
	"time"
)

// A decision model ("karar modeli") is a user-configured entry, like a provider
// instance: a backend plus the endpoint, credentials and model id it runs with.
// There can be many — Jev through OpenRouter, an OpenJev server on the local
// GPU, a small Ollama model — and each authority picks the one it uses.

// CredentialSource says where a decision model gets its credentials.
type CredentialSource string

const (
	// CredentialsOwn: the model carries its own base URL and (optional) API
	// key, stored encrypted like a provider secret.
	CredentialsOwn CredentialSource = "own"
	// CredentialsProvider: the model borrows the key and base URL of a provider
	// instance (an OpenRouter account the user already configured), so the
	// key is never entered twice.
	CredentialsProvider CredentialSource = "provider"
)

// SecretAPIKey is the secret holding a model's own API key.
const SecretAPIKey = "key"

// Bounds of a model's context size override.
const (
	minContextTokens = 1024
	maxContextTokens = 1_000_000
)

// ModelInstance is one configured decision model. SecretsEnc is never
// serialised to the API (ToDTO exposes only which secrets are set).
type ModelInstance struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Backend string `json:"backend"`
	Enabled bool   `json:"enabled"`
	// Model is the service model id ("typesafe/jev-1.13", "qwen3:4b").
	Model       string           `json:"model"`
	Credentials CredentialSource `json:"credentials"`
	// ProviderInstanceID is the provider whose credentials are borrowed
	// (CredentialsProvider). "" = the first enabled one the backend accepts.
	ProviderInstanceID string `json:"providerInstanceId,omitempty"`
	// BaseURL is the model's own endpoint (CredentialsOwn). "" = the
	// backend's default.
	BaseURL string `json:"baseUrl,omitempty"`
	// TimeoutMs bounds one attempt of a call.
	TimeoutMs int `json:"timeoutMs"`
	// ContextTokens overrides the backend's context size (0 = the backend's).
	ContextTokens int `json:"contextTokens,omitempty"`
	// Config holds the backend-specific fields (Manifest.Fields).
	Config     map[string]string `json:"config,omitempty"`
	SecretsEnc map[string]string `json:"-"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

// Timeout is the per-attempt timeout.
func (m ModelInstance) Timeout() time.Duration {
	if m.TimeoutMs <= 0 {
		return DefaultTimeout
	}
	return time.Duration(m.TimeoutMs) * time.Millisecond
}

// ModelDTO is the masked, client-facing view of a ModelInstance.
type ModelDTO struct {
	ID                 string            `json:"id"`
	Label              string            `json:"label"`
	Backend            string            `json:"backend"`
	Enabled            bool              `json:"enabled"`
	Model              string            `json:"model"`
	Credentials        CredentialSource  `json:"credentials"`
	ProviderInstanceID string            `json:"providerInstanceId"`
	BaseURL            string            `json:"baseUrl"`
	TimeoutMs          int               `json:"timeoutMs"`
	ContextTokens      int               `json:"contextTokens"`
	Config             map[string]string `json:"config"`
	SecretsSet         map[string]bool   `json:"secretsSet"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

// ToDTO projects the model into its masked view.
func (m ModelInstance) ToDTO() ModelDTO {
	set := make(map[string]bool, len(m.SecretsEnc))
	for k, v := range m.SecretsEnc {
		set[k] = v != ""
	}
	cfg := maps.Clone(m.Config)
	if cfg == nil {
		cfg = map[string]string{}
	}
	return ModelDTO{
		ID:                 m.ID,
		Label:              m.Label,
		Backend:            m.Backend,
		Enabled:            m.Enabled,
		Model:              m.Model,
		Credentials:        m.Credentials,
		ProviderInstanceID: m.ProviderInstanceID,
		BaseURL:            m.BaseURL,
		TimeoutMs:          m.TimeoutMs,
		ContextTokens:      m.ContextTokens,
		Config:             cfg,
		SecretsSet:         set,
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
	}
}

// ModelInput is the create/update request. Secrets is write-only plaintext with
// the provider convention: a key absent from the map keeps the stored secret,
// an empty value clears it, a non-empty value replaces it.
type ModelInput struct {
	ID                 string            `json:"id"`
	Label              string            `json:"label"`
	Backend            string            `json:"backend"`
	Enabled            bool              `json:"enabled"`
	Model              string            `json:"model"`
	Credentials        CredentialSource  `json:"credentials"`
	ProviderInstanceID string            `json:"providerInstanceId"`
	BaseURL            string            `json:"baseUrl"`
	TimeoutMs          int               `json:"timeoutMs"`
	ContextTokens      int               `json:"contextTokens"`
	Config             map[string]string `json:"config"`
	Secrets            map[string]string `json:"secrets"`
}

// configValidator is implemented by backends whose fields need more than the
// manifest's shape check (a JSON field, number ranges).
type configValidator interface {
	ValidateConfig(cfg map[string]string) error
}

// normalizeModelInput checks a create/update request against its backend and
// fills defaults. existing is the stored model on update (nil on create).
func normalizeModelInput(in ModelInput, existing *ModelInstance) (ModelInput, Manifest, error) {
	in.Backend = strings.TrimSpace(in.Backend)
	if existing != nil {
		if in.Backend == "" {
			in.Backend = existing.Backend
		}
		if in.Backend != existing.Backend {
			return in, Manifest{}, fmt.Errorf("decision model %q: the backend cannot be changed (has %q, got %q)", existing.ID, existing.Backend, in.Backend)
		}
	}
	b, ok := Lookup(in.Backend)
	if !ok {
		return in, Manifest{}, fmt.Errorf("unknown decision backend %q", in.Backend)
	}
	m := b.Manifest()

	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" {
		in.Label = m.Label
	}
	in.Model = strings.TrimSpace(in.Model)
	if in.Model == "" {
		in.Model = m.DefaultModel
	}
	if in.Model == "" {
		return in, m, fmt.Errorf("decision model %q needs a model id", in.Label)
	}

	switch in.Credentials {
	case "":
		in.Credentials = CredentialsOwn
	case CredentialsOwn, CredentialsProvider:
	default:
		return in, m, fmt.Errorf("unknown credential source %q (own or provider)", in.Credentials)
	}
	in.ProviderInstanceID = strings.TrimSpace(in.ProviderInstanceID)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if in.Credentials == CredentialsProvider {
		if len(m.ProviderKinds) == 0 {
			return in, m, fmt.Errorf("%s cannot borrow provider credentials", m.Label)
		}
		// A borrowed key only ever goes to the provider's own base URL.
		in.BaseURL = ""
	} else {
		in.ProviderInstanceID = ""
		if in.BaseURL != "" && !strings.HasPrefix(in.BaseURL, "http://") && !strings.HasPrefix(in.BaseURL, "https://") {
			return in, m, fmt.Errorf("base URL %q must start with http:// or https://", in.BaseURL)
		}
	}

	if in.TimeoutMs <= 0 {
		in.TimeoutMs = m.DefaultTimeoutMs
	}
	if in.TimeoutMs <= 0 {
		in.TimeoutMs = int(DefaultTimeout.Milliseconds())
	}
	in.TimeoutMs = min(max(in.TimeoutMs, minTimeoutMs), maxTimeoutMs)
	if in.ContextTokens < 0 {
		in.ContextTokens = 0
	}
	if in.ContextTokens > 0 {
		in.ContextTokens = min(max(in.ContextTokens, minContextTokens), maxContextTokens)
	}

	cfg := make(map[string]string, len(in.Config))
	for k, v := range in.Config {
		if _, ok := m.FieldByKey(k); !ok {
			return in, m, fmt.Errorf("unknown field %q for %s", k, m.Label)
		}
		if v = strings.TrimSpace(v); v != "" {
			cfg[k] = v
		}
	}
	in.Config = cfg
	if cv, ok := b.(configValidator); ok {
		if err := cv.ValidateConfig(cfg); err != nil {
			return in, m, err
		}
	}

	for k := range in.Secrets {
		if k != SecretAPIKey {
			return in, m, fmt.Errorf("unknown secret %q (only %q)", k, SecretAPIKey)
		}
	}
	if in.Credentials == CredentialsOwn && m.KeyRequired && !keyWillBeSet(in, existing) {
		return in, m, fmt.Errorf("%s needs an API key", m.Label)
	}
	return in, m, nil
}

// keyWillBeSet reports whether the model ends up with an API key after in is
// applied (a new value, or a stored one the request leaves alone).
func keyWillBeSet(in ModelInput, existing *ModelInstance) bool {
	if v, present := in.Secrets[SecretAPIKey]; present {
		return v != ""
	}
	return existing != nil && existing.SecretsEnc[SecretAPIKey] != ""
}
