package providers

import "fmt"

// deepseekAnthropicMessagesURL is DeepSeek's Anthropic-compatible Messages
// endpoint (documented at https://api.deepseek.com/anthropic). It accepts the
// Anthropic request shape (tool-use, extended thinking, image content, x-api-key
// auth) and returns native Messages-format responses. cache_control breakpoints
// are accepted but ignored — DeepSeek caches automatically.
const deepseekAnthropicMessagesURL = "https://api.deepseek.com/anthropic/v1/messages"

// deepseekAnthropicKind routes DeepSeek models through the Anthropic Messages
// transport instead of the OpenAI-compatible one. Unlike the "deepseek" kind,
// this drives the full agentic protocol via the shared anthropic.go client,
// surfacing native tool-use and extended thinking. It reuses the DeepSeek API
// key (no separate credential) and the same model list, mirroring
// minimax-anthropic.
//
// DeepSeek's Messages endpoint ignores thinking.budget_tokens: depth is read
// from output_config.effort (low/high/max), which thinkingFor sends for the V4
// family (see coarseEffortThinking). An unknown model id is silently served by
// deepseek-flash there, so a typo does not fail — it just runs V4.1 Flash.
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
			Transport:        TransportAPI,
			Multi:            true,
			Fields: []FieldSpec{
				{Key: FieldKeyAPIKey, Label: "API Anahtari", Type: "password", Required: true, Secret: true, Help: "DeepSeek hesap API anahtari (deepseek kind'iyla ayni anahtar)."},
			},
			Models: deepseekModels,
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
			return NewAnthropic(cfg.Key).WithEndpoint("deepseek-anthropic", endpoint, deepseekDefaultModel), nil
		},
	))
}
