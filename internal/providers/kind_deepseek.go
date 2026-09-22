package providers

import "fmt"

// deepseekBaseURL is DeepSeek's OpenAI-compatible Chat Completions base. The
// client appends "/chat/completions"; the "/v1" path segment DeepSeek documents
// is optional and unrelated to the model version, so the bare host is used.
const deepseekBaseURL = "https://api.deepseek.com"

// deepseekDefaultModel is applied when a request omits a model. "deepseek-flash"
// is DeepSeek-V4.1-Flash (2026-09-10): the high-volume, low-cost tier and the
// sensible default.
const deepseekDefaultModel = "deepseek-flash"

// deepseekModels is the curated DeepSeek lineup, shared by the "deepseek" and
// "deepseek-anthropic" kinds (same key, same models, different protocol). Both
// current models reason by default and take a low/high/max effort (see
// UsesCoarseEffort).
//
// "deepseek-v4-flash" stays listed only as the legacy name DeepSeek keeps routing
// to V4.1 Flash (at V4.1 Flash prices) so existing agents keep working; it is a
// temporary alias, not a separate model. The aliases deepseek-chat /
// deepseek-reasoner were retired 2026-07-24 and are intentionally omitted;
// AllowCustomModel still lets a user type any id.
var deepseekModels = []ModelInfo{
	{ID: "deepseek-flash", Label: "DeepSeek V4.1 Flash — hızlı/ucuz", Description: "1M bağlam; düşünme kapalı ya da low/high/max ($0.15/$0.60 · 1M, cache-hit $0.003; peak saatlerde 2×)"},
	{ID: "deepseek-v4-pro", Label: "DeepSeek V4 Pro — güçlü/akıl-yürütme", Description: "V4-Pro-0813 · 1M bağlam; düşünme kapalı ya da low/high/max ($0.66/$1.98 · 1M, cache-hit $0.022; peak saatlerde 2×)"},
	{ID: "deepseek-v4-flash", Label: "DeepSeek V4 Flash — eski ad", Description: "Geçici takma ad: DeepSeek bunu V4.1 Flash'a yönlendiriyor (Flash fiyatı); yeni ajanlarda deepseek-flash seçin"},
}

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
// The client opts into reasoning_effort AND the explicit thinking switch: DeepSeek
// reasons by default, so "off" (and every tool-loop turn, which runs with a zero
// thinking budget) must send thinking {type:"disabled"} — otherwise DeepSeek
// demands the earlier reasoning_content back on the next tool call.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "deepseek",
			AppliesToolHooks: true,
			Label:            "DeepSeek (OpenAI-uyumlu)",
			NeedsKey:         true,
			NeedsBaseURL:     true,
			AllowCustomModel: true,
			Order:            6,
			Transport:        TransportAPI,
			Multi:            true,
			Fields: []FieldSpec{
				{Key: FieldKeyAPIKey, Label: "API Anahtari", Type: "password", Required: true, Secret: true, Help: "DeepSeek hesap API anahtari."},
				{Key: FieldKeyBaseURL, Label: "Taban URL", Type: "text", Placeholder: deepseekBaseURL, Help: "Bos birakilirsa DeepSeek'in varsayilan API uc noktasi kullanilir."},
			},
			Models: deepseekModels,
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
			return NewOpenAICompat("deepseek", cfg.Key, base, deepseekDefaultModel).
				WithCaps(true, "").
				WithThinkingToggle(), nil
		},
	))
}
