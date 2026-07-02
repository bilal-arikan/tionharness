package providers

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// CustomSpec is a user-configured OpenAI- or Anthropic-compatible provider, fed
// to the registry from settings (Key is the decrypted plaintext). Its ID becomes
// a selectable provider identifier alongside the built-in kinds.
type CustomSpec struct {
	ID           string
	Label        string
	Kind         string // "openai" | "anthropic"
	BaseURL      string
	DefaultModel string
	Models       string // optional model-id suggestions (comma/newline)
	Key          string
	// Reasoning: send reasoning_effort (openai kind) mapped from ThinkingBudget.
	Reasoning bool
	// PromptCache: "native" | "auto" | "none" | "" — "native" injects an
	// Anthropic-style cache_control breakpoint on the system prefix (openai kind).
	PromptCache string
}

// Registry builds providers by name using configured credentials and
// locally-available CLI tools. Its fields are mutable at runtime so the
// Settings screen can update the Anthropic key or claude CLI path live.
type Registry struct {
	mu                 sync.RWMutex
	anthropicKey       string
	claudeCLIPath      string // resolved path to `claude` binary, or "" if absent
	claudeConfigDir    string // CLAUDE_CONFIG_DIR override for claude-cli, or "" to inherit ~/.claude
	claudeAuthKind     string // claude-cli credential kind: "oauth" | "apikey" | ""
	claudeAuthToken    string // claude-cli credential value injected into the subprocess env
	defaultModel       string // applied when a request leaves Model empty

	betaExtendedCache bool // anthropic extended prompt-cache TTL beta

	minimaxKey     string // MiniMax (OpenAI-compatible) API key
	minimaxBaseURL string // MiniMax base URL ("" = public default)

	openrouterKey     string // OpenRouter (OpenAI-compatible) API key
	openrouterBaseURL string // OpenRouter base URL ("" = public default)

	custom      map[string]CustomSpec // user-added providers, keyed by id
	customOrder []string              // ids in catalog order
}

// NewRegistry creates a registry. It auto-detects the claude CLI on PATH so the
// keyless claude-cli provider works out of the box; Settings can later override
// paths and keys.
func NewRegistry(anthropicKey string) *Registry {
	claudePath, _ := exec.LookPath("claude")
	return &Registry{
		anthropicKey:  anthropicKey,
		claudeCLIPath: claudePath,
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

// SetClaudeConfigDir overrides CLAUDE_CONFIG_DIR for claude-cli subprocesses. An
// empty value inherits the ambient ~/.claude (default behaviour).
func (r *Registry) SetClaudeConfigDir(dir string) {
	r.mu.Lock()
	r.claudeConfigDir = dir
	r.mu.Unlock()
}

// SetClaudeAuth sets the credential injected into claude-cli subprocesses. kind is
// "oauth" (→ CLAUDE_CODE_OAUTH_TOKEN) or "apikey" (→ ANTHROPIC_API_KEY); an empty
// token or kind injects nothing (the CLI falls back to its config-dir login).
func (r *Registry) SetClaudeAuth(token, kind string) {
	r.mu.Lock()
	r.claudeAuthToken = token
	r.claudeAuthKind = kind
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
func (r *Registry) SetAnthropicBetas(extendedCache bool) {
	r.mu.Lock()
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

// SetOpenRouter updates the OpenRouter (OpenAI-compatible) API key and base URL.
func (r *Registry) SetOpenRouter(key, baseURL string) {
	r.mu.Lock()
	r.openrouterKey = key
	r.openrouterBaseURL = baseURL
	r.mu.Unlock()
}

// OpenRouterConfigured reports whether an OpenRouter key is set.
func (r *Registry) OpenRouterConfigured() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.openrouterKey != ""
}

// SetCustomProviders replaces the set of user-added providers (called from
// applySettings whenever settings change).
func (r *Registry) SetCustomProviders(list []CustomSpec) {
	m := make(map[string]CustomSpec, len(list))
	order := make([]string, 0, len(list))
	for _, c := range list {
		if c.ID == "" {
			continue
		}
		m[c.ID] = c
		order = append(order, c.ID)
	}
	r.mu.Lock()
	r.custom = m
	r.customOrder = order
	r.mu.Unlock()
}

// CustomCatalog returns catalog entries for the user-added providers, in the
// order they were registered. Availability is layered on by the API handler.
func (r *Registry) CustomCatalog() []CatalogEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CatalogEntry, 0, len(r.customOrder))
	for _, id := range r.customOrder {
		c := r.custom[id]
		out = append(out, CatalogEntry{
			ID:               c.ID,
			Label:            c.Label,
			NeedsKey:         true,
			AllowCustomModel: true,
			Models:           parseModelList(c.Models),
		})
	}
	return out
}

// parseModelList turns a comma/newline separated id list into ModelInfo entries
// (label = id). Blank entries are skipped.
func parseModelList(s string) []ModelInfo {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
	out := make([]ModelInfo, 0, len(fields))
	for _, f := range fields {
		id := strings.TrimSpace(f)
		if id != "" {
			out = append(out, ModelInfo{ID: id, Label: id})
		}
	}
	return out
}

// buildCustom constructs a provider from a CustomSpec, choosing the transport
// by Kind.
func buildCustom(c CustomSpec) (Provider, error) {
	if c.Key == "" {
		return nil, fmt.Errorf("provider %q not configured (set an API key in Settings)", c.ID)
	}
	if c.Kind == "anthropic" {
		endpoint := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/") + "/messages"
		// Anthropic-protocol endpoints support thinking + cache_control natively;
		// no extra capability wiring needed here.
		return NewAnthropic(c.Key).WithEndpoint(c.ID, endpoint, c.DefaultModel), nil
	}
	return NewOpenAICompat(c.ID, c.Key, c.BaseURL, c.DefaultModel).
		WithCaps(c.Reasoning, c.PromptCache), nil
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
		Model:              r.defaultModel,
		CLIPath:            r.claudeCLIPath,
		CLIConfigDir:       r.claudeConfigDir,
		CLIAuthKind:        r.claudeAuthKind,
		CLIAuthToken:       r.claudeAuthToken,
		ExtendedCache:      r.betaExtendedCache,
	}
	switch id {
	case "anthropic":
		cfg.Key = r.anthropicKey
	case "minimax":
		cfg.Key = r.minimaxKey
		cfg.BaseURL = r.minimaxBaseURL
	case "minimax-anthropic":
		// Reuses the MiniMax key but the Anthropic-compatible endpoint; the kind
		// supplies its own base URL (minimaxBaseURL is the OpenAI base, N/A here).
		cfg.Key = r.minimaxKey
	case "openrouter":
		cfg.Key = r.openrouterKey
		cfg.BaseURL = r.openrouterBaseURL
	}
	return cfg
}

// Available reports whether the provider id is registered and usable with the
// current configuration (key set / CLI present). Used by the catalog handler.
func (r *Registry) Available(id string) bool {
	if k, ok := lookupKind(id); ok {
		return k.Available(r.resolve(id))
	}
	r.mu.RLock()
	c, ok := r.custom[id]
	r.mu.RUnlock()
	return ok && c.Key != ""
}

// Get returns a provider for the given name, or an error if unsupported
// or unconfigured. It dispatches through the registered provider kinds first,
// then user-added custom providers; the empty name maps to the keyless
// claude-cli default.
func (r *Registry) Get(name string) (Provider, error) {
	if k, ok := lookupKind(name); ok {
		return k.Build(r.resolve(name))
	}
	r.mu.RLock()
	c, ok := r.custom[name]
	r.mu.RUnlock()
	if ok {
		return buildCustom(c)
	}
	return nil, fmt.Errorf("unknown provider: %q", name)
}
