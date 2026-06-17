package providers

import (
	"fmt"
	"os/exec"
	"sync"
)

// Registry builds providers by name using configured credentials and
// locally-available CLI tools. Its fields are mutable at runtime so the
// Settings screen can update the Anthropic key or claude CLI path live.
type Registry struct {
	mu            sync.RWMutex
	anthropicKey  string
	claudeCLIPath string // resolved path to `claude` binary, or "" if absent
	defaultModel  string // applied when a request leaves Model empty

	betaOneMContext   bool // anthropic 1M-context beta
	betaExtendedCache bool // anthropic extended prompt-cache TTL beta

	minimaxKey     string // MiniMax (OpenAI-compatible) API key
	minimaxBaseURL string // MiniMax base URL ("" = public default)
}

// NewRegistry creates a registry. It auto-detects the claude CLI on PATH.
func NewRegistry(anthropicKey string) *Registry {
	path, _ := exec.LookPath("claude")
	return &Registry{
		anthropicKey:  anthropicKey,
		claudeCLIPath: path,
	}
}

// SetAnthropicKey updates the API key used by the anthropic provider.
func (r *Registry) SetAnthropicKey(key string) {
	r.mu.Lock()
	r.anthropicKey = key
	r.mu.Unlock()
}

// SetClaudeCLIPath overrides the claude binary path. An empty value re-runs
// PATH auto-detection so clearing the override restores default behaviour.
func (r *Registry) SetClaudeCLIPath(path string) {
	if path == "" {
		path, _ = exec.LookPath("claude")
	}
	r.mu.Lock()
	r.claudeCLIPath = path
	r.mu.Unlock()
}

// SetDefaultModel sets the model applied when a request omits one.
func (r *Registry) SetDefaultModel(model string) {
	r.mu.Lock()
	r.defaultModel = model
	r.mu.Unlock()
}

// SetAnthropicBetas toggles the optional Anthropic beta capabilities applied to
// anthropic provider instances.
func (r *Registry) SetAnthropicBetas(oneMContext, extendedCache bool) {
	r.mu.Lock()
	r.betaOneMContext = oneMContext
	r.betaExtendedCache = extendedCache
	r.mu.Unlock()
}

// SetMinimax updates the MiniMax (OpenAI-compatible) API key and base URL.
func (r *Registry) SetMinimax(key, baseURL string) {
	r.mu.Lock()
	r.minimaxKey = key
	r.minimaxBaseURL = baseURL
	r.mu.Unlock()
}

// MinimaxConfigured reports whether a MiniMax key is set.
func (r *Registry) MinimaxConfigured() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.minimaxKey != ""
}

// ClaudeCLIAvailable reports whether the claude CLI was found.
func (r *Registry) ClaudeCLIAvailable() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.claudeCLIPath != ""
}

// AnthropicConfigured reports whether an Anthropic key is set.
func (r *Registry) AnthropicConfigured() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.anthropicKey != ""
}

// resolve assembles the ResolvedConfig a kind needs from the registry's live
// fields. This per-id mapping of credentials onto the common config is the one
// remaining id-aware seam; the data-driven instance model (Faz 2) replaces the
// typed key fields with a generic per-instance credential store and drops it.
func (r *Registry) resolve(id string) ResolvedConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg := ResolvedConfig{
		Model:         r.defaultModel,
		CLIPath:       r.claudeCLIPath,
		OneMContext:   r.betaOneMContext,
		ExtendedCache: r.betaExtendedCache,
	}
	switch id {
	case "anthropic":
		cfg.Key = r.anthropicKey
	case "minimax":
		cfg.Key = r.minimaxKey
		cfg.BaseURL = r.minimaxBaseURL
	}
	return cfg
}

// Available reports whether the provider id is registered and usable with the
// current configuration (key set / CLI present). Used by the catalog handler.
func (r *Registry) Available(id string) bool {
	k, ok := lookupKind(id)
	if !ok {
		return false
	}
	return k.Available(r.resolve(id))
}

// Get returns a provider for the given name, or an error if unsupported
// or unconfigured. It dispatches through the registered provider kinds; the
// empty name maps to the keyless claude-cli default.
func (r *Registry) Get(name string) (Provider, error) {
	k, ok := lookupKind(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider: %q", name)
	}
	return k.Build(r.resolve(name))
}
