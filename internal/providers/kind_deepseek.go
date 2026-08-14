package providers

import "fmt"

// deepseekBaseURL is DeepSeek's OpenAI-compatible Chat Completions base. The
// client appends "/chat/completions"; the "/v1" path segment DeepSeek documents
// is optional and unrelated to the model version, so the bare host is used.
const deepseekBaseURL = "https://api.deepseek.com"

// deepseekDefaultModel is applied when a request omits a model. V4 Flash is the
// high-volume, low-cost tier and the sensible default.
const deepseekDefaultModel = "deepseek-v4-flash"

// deepseekKind is the DeepSeek transport: a first-party OpenAI-compatible
// endpoint for the DeepSeek V4 family, reusing the shared OpenAICompat client
// (tool-use + streaming) so DeepSeek models drive the native agentic loop like
// MiniMax and OpenRouter. It uses its own DeepSeek API key.
//
// Like the other OpenAI-compatible kinds, this is a self-registering plugin
// file — adding it touches no other provider code; the catalog, registry
// resolve() and settings wiring pick it up via its Manifest and a dedicated key
// field.
//
// The legacy aliases deepseek-chat / deepseek-reasoner were retired 2026-07-24
// and are intentionally omitted; AllowCustomModel still lets a user type any id.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "deepseek",
			Label:            "DeepSeek (OpenAI-uyumlu)",
			NeedsKey:         true,
			NeedsBaseURL:     true,
			AllowCustomModel: true,
			Order:            6,
			Models: []ModelInfo{
				{ID: "deepseek-v4-flash", Label: "DeepSeek V4 Flash — hızlı/ucuz", Description: "1M bağlam, yüksek hacim; düşünmeyen mod ($0.22/$0.66 · 1M, cache-hit ~$0.007; peak saatlerde 2×)"},
				{ID: "deepseek-v4-pro", Label: "DeepSeek V4 Pro — güçlü/akıl-yürütme", Description: "1M bağlam, düşünme ağırlıklı katman ($0.66/$1.98 · 1M, cache-hit ~$0.022; peak saatlerde 2×)"},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.Key != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.Key == "" {
				return nil, fmt.Errorf("deepseek provider not configured (set the DeepSeek API key in Settings)")
			}
			base := cfg.BaseURL
			if base == "" {
				base = deepseekBaseURL
			}
			return NewOpenAICompat("deepseek", cfg.Key, base, deepseekDefaultModel), nil
		},
	))
}
