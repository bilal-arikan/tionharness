package providers

import "fmt"

// zaiAnthropicMessagesURL is Z.ai's Anthropic-compatible Messages endpoint. It
// accepts the same request shape as Anthropic (tool-use, streaming, extended
// thinking, cache_control breakpoints, x-api-key auth) and returns native
// Messages-format responses — the GLM Coding Plan exposes this as a Claude Code
// drop-in. The base URL published by Z.ai is ".../api/anthropic"; Anthropic
// clients append "/v1/messages".
const zaiAnthropicMessagesURL = "https://api.z.ai/api/anthropic/v1/messages"

const zaiDefaultModel = "glm-5.2"

// zaiKind routes Z.ai's GLM family through the Anthropic Messages transport,
// unlocking native tool-use and extended thinking via the shared anthropic.go
// client (like minimax-anthropic). It uses its own Z.ai API key.
//
// Z.ai also offers an OpenAI-compatible endpoint, but the Anthropic protocol is
// preferred here because the shared client drives the full agentic loop and GLM
// on the Anthropic endpoint reports tool-use + thinking natively.
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "zai",
			AppliesToolHooks: true,
			Label:            "Z.ai GLM (Anthropic modu · tool-use + thinking)",
			NeedsKey:         true,
			NeedsBaseURL:     true,
			AllowCustomModel: true,
			Order:            5,
			Models: []ModelInfo{
				{ID: "glm-5.2", Label: "GLM-5.2 — güncel amiral", Description: "Anthropic modu: araç kullanımı + düşünme ($1.40/$4.40 · 1M)"},
				{ID: "glm-5.1", Label: "GLM-5.1 — önceki nesil", Description: "Anthropic modu: araç kullanımı + düşünme ($0.97/$3.04 · 1M)"},
				{ID: "glm-5", Label: "GLM-5 — dengeli/ucuz taban", Description: "Anthropic modu ($0.60/$1.92 · 1M)"},
				{ID: "glm-4.7", Label: "GLM-4.7 — önceki kodlama modeli", Description: "Anthropic modu: araç kullanımı ($0.60/$2.20 · 1M)"},
				{ID: "glm-4.7-flash", Label: "GLM-4.7 Flash — hızlı/çok ucuz", Description: "Düşük gecikme, yüksek hacim ($0.06/$0.40 · 1M)"},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.Key != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.Key == "" {
				return nil, fmt.Errorf("zai provider not configured (set the Z.ai API key in Settings)")
			}
			endpoint := cfg.BaseURL
			if endpoint == "" {
				endpoint = zaiAnthropicMessagesURL
			}
			// No betas: the anthropic-beta flags are Anthropic-host-specific (Z.ai
			// accepts but ignores them); leave them off to avoid implying support.
			return NewAnthropic(cfg.Key).WithEndpoint("zai", endpoint, zaiDefaultModel), nil
		},
	))
}
