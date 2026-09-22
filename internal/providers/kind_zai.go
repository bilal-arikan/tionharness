package providers

import "fmt"

// zaiAnthropicMessagesURL is Z.ai's Anthropic-compatible Messages endpoint. It
// accepts the same request shape as Anthropic (tool-use, streaming, extended
// thinking, cache_control breakpoints, x-api-key auth) and returns native
// Messages-format responses — the GLM Coding Plan exposes this as a Claude Code
// drop-in. The base URL published by Z.ai is ".../api/anthropic"; Anthropic
// clients append "/v1/messages".
const zaiAnthropicMessagesURL = "https://api.z.ai/api/anthropic/v1/messages"

// zaiDefaultModel is applied when a request omits a model: GLM-5.3 (2026-08),
// the current flagship.
const zaiDefaultModel = "glm-5.3"

// zaiKind routes Z.ai's GLM family through the Anthropic Messages transport,
// unlocking native tool-use and extended thinking via the shared anthropic.go
// client (like minimax-anthropic). It uses its own Z.ai API key.
//
// Z.ai also offers an OpenAI-compatible endpoint, but the Anthropic protocol is
// preferred here because the shared client drives the full agentic loop and GLM
// on the Anthropic endpoint reports tool-use + thinking natively.
//
// The GLM-5.3 family (glm-5.3, glm-5.3-flash, glm-5.3-flashx) always reasons —
// thinking cannot be disabled — and its depth is output_config.effort
// (low/high/max, server default max; budget_tokens is ignored). thinkingFor sends
// that effort, and "off" becomes the lowest effort (see coarseEffortThinking).
// The long per-request budget applies to the family (LongRequestModel): a single
// max-effort call can run for minutes.
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
			Transport:        TransportAPI,
			Multi:            true,
			Fields: []FieldSpec{
				{Key: FieldKeyAPIKey, Label: "API Anahtari", Type: "password", Required: true, Secret: true, Help: "Z.ai hesap API anahtari."},
				{Key: FieldKeyBaseURL, Label: "Taban URL", Type: "text", Placeholder: zaiAnthropicMessagesURL, Help: "Bos birakilirsa Z.ai'nin Anthropic-uyumlu uc noktasi kullanilir."},
			},
			Models: []ModelInfo{
				{ID: "glm-5.3", Label: "GLM-5.3 — güncel amiral", Description: "Anthropic modu: araç kullanımı + düşünme (daima açık, low/high/max) ($1.40/$4.40 · 1M, cache $0.26)"},
				{ID: "glm-5.3-flash", Label: "GLM-5.3 Flash — hızlı/ucuz, çok-kipli", Description: "Daima düşünür; Coding Plan'da 3× kota ($0.15/$0.50 · 1M, cache $0.03)"},
				{ID: "glm-5.3-flashx", Label: "GLM-5.3 FlashX — en hızlı", Description: "Flash'ın ~200 token/sn katmanı; henüz Coding Plan'da yok ($0.37/$1.25 · 1M, cache $0.075)"},
				{ID: "glm-5.2", Label: "GLM-5.2 — önceki amiral", Description: "Anthropic modu: araç kullanımı + düşünme ($1.40/$4.40 · 1M)"},
				{ID: "glm-5.1", Label: "GLM-5.1 — önceki nesil", Description: "Anthropic modu: araç kullanımı + düşünme ($1.40/$4.40 · 1M)"},
				{ID: "glm-5", Label: "GLM-5 — dengeli taban", Description: "Anthropic modu ($1.00/$3.20 · 1M)"},
				{ID: "glm-4.7", Label: "GLM-4.7 — önceki kodlama modeli", Description: "Anthropic modu: araç kullanımı ($0.60/$2.20 · 1M)"},
				{ID: "glm-4.7-flash", Label: "GLM-4.7 Flash — ücretsiz", Description: "Düşük gecikme, yüksek hacim (Z.ai'de ücretsiz)"},
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
