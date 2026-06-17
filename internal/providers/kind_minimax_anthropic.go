package providers

import "fmt"

// minimaxAnthropicMessagesURL is MiniMax's Anthropic-compatible Messages
// endpoint. It accepts the same request shape as Anthropic (tool-use, extended
// thinking, cache_control breakpoints, x-api-key auth) and returns native
// Messages-format responses — verified live against MiniMax-M2.1.
const minimaxAnthropicMessagesURL = "https://api.minimax.io/anthropic/v1/messages"

const minimaxAnthropicDefaultModel = "MiniMax-M2.1"

// minimaxAnthropicKind routes MiniMax models through the Anthropic Messages
// transport instead of the OpenAI-compatible one. Unlike the "minimax" kind
// (text-only), this unlocks native tool-use and extended thinking, since the
// shared anthropic.go client drives the full agentic protocol. It reuses the
// MiniMax API key (no separate credential).
//
// This kind is the first payoff of the plugin foundation: a fourth transport
// added as one self-registering file, with no edits to the registry, catalog
// or API.
type minimaxAnthropicKind struct{}

func (minimaxAnthropicKind) Manifest() Manifest {
	return Manifest{
		Kind:             "minimax-anthropic",
		Label:            "MiniMax (Anthropic modu · tool-use + thinking)",
		NeedsKey:         true,
		NeedsBaseURL:     true,
		AllowCustomModel: true,
		Order:            3,
		Models: []ModelInfo{
			{ID: "MiniMax-M2.1", Label: "MiniMax M2.1 — çok dilli kodlama", Description: "Anthropic modu: araç kullanımı + düşünme"},
			{ID: "MiniMax-M2.1-lightning", Label: "MiniMax M2.1 Lightning — hızlı", Description: "M2.1'in hızlı varyantı"},
			{ID: "MiniMax-M2", Label: "MiniMax M2 — ajan/akıl yürütme", Description: "Ajan yetenekleri + gelişmiş akıl yürütme"},
		},
	}
}

func (minimaxAnthropicKind) Available(cfg ResolvedConfig) bool { return cfg.Key != "" }

func (minimaxAnthropicKind) Build(cfg ResolvedConfig) (Provider, error) {
	if cfg.Key == "" {
		return nil, fmt.Errorf("minimax-anthropic provider not configured (set the MiniMax API key in Settings)")
	}
	endpoint := cfg.BaseURL
	if endpoint == "" {
		endpoint = minimaxAnthropicMessagesURL
	}
	// No betas: the anthropic-beta flags are Anthropic-host-specific (MiniMax
	// accepts but ignores them); leave them off to avoid implying support.
	return NewAnthropic(cfg.Key).WithEndpoint("minimax-anthropic", endpoint, minimaxAnthropicDefaultModel), nil
}

func init() { RegisterKind(minimaxAnthropicKind{}) }
