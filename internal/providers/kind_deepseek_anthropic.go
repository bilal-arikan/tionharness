package providers

import "fmt"

// deepseekAnthropicMessagesURL is DeepSeek's Anthropic-compatible Messages
// endpoint (documented at https://api.deepseek.com/anthropic). It accepts the
// Anthropic request shape (tool-use, extended thinking, cache_control
// breakpoints, x-api-key auth) and returns native Messages-format responses.
const deepseekAnthropicMessagesURL = "https://api.deepseek.com/anthropic/v1/messages"

const deepseekAnthropicDefaultModel = "deepseek-v4-flash"

// deepseekAnthropicKind routes DeepSeek models through the Anthropic Messages
// transport instead of the OpenAI-compatible one. Unlike the "deepseek" kind,
// this drives the full agentic protocol via the shared anthropic.go client,
// surfacing native tool-use and extended thinking — the payoff for the
// reasoning-heavy deepseek-v4-pro. It reuses the DeepSeek API key (no separate
// credential), mirroring minimax-anthropic.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "deepseek-anthropic",
			AppliesToolHooks: true,
			Label:            "DeepSeek (Anthropic modu · tool-use + thinking)",
			NeedsKey:         true,
			NeedsBaseURL:     true,
			AllowCustomModel: true,
			Order:            7,
			Models: []ModelInfo{
				{ID: "deepseek-v4-flash", Label: "DeepSeek V4 Flash — hızlı/ucuz", Description: "Anthropic modu: araç kullanımı ($0.22/$0.66 · 1M, peak saatlerde 2×)"},
				{ID: "deepseek-v4-pro", Label: "DeepSeek V4 Pro — güçlü/akıl-yürütme", Description: "Anthropic modu: araç kullanımı + düşünme ($0.66/$1.98 · 1M, peak saatlerde 2×)"},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.Key != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.Key == "" {
				return nil, fmt.Errorf("deepseek-anthropic provider not configured (set the DeepSeek API key in Settings)")
			}
			endpoint := cfg.BaseURL
			if endpoint == "" {
				endpoint = deepseekAnthropicMessagesURL
			}
			// No betas: the anthropic-beta flags are Anthropic-host-specific (DeepSeek
			// accepts but ignores them); leave them off to avoid implying support.
			return NewAnthropic(cfg.Key).WithEndpoint("deepseek-anthropic", endpoint, deepseekAnthropicDefaultModel), nil
		},
	))
}
