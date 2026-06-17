package providers

import "fmt"

// minimaxKind is the MiniMax (OpenAI-compatible Chat Completions) transport.
// The underlying client works against any OpenAI-compatible endpoint via a
// custom base URL, so this kind doubles as a generic OpenAI-style provider.
// Text completion only — no tool-use (see minimax.go).
type minimaxKind struct{}

func (minimaxKind) Manifest() Manifest {
	return Manifest{
		Kind:             "minimax",
		Label:            "MiniMax (OpenAI-uyumlu)",
		NeedsKey:         true,
		NeedsBaseURL:     true,
		AllowCustomModel: true,
		Order:            2,
		Models: []ModelInfo{
			{ID: "MiniMax-M2.1", Label: "MiniMax M2.1 — çok dilli kodlama", Description: "Güçlü kodlama, çok dil (~60 tps)"},
			{ID: "MiniMax-M2.1-lightning", Label: "MiniMax M2.1 Lightning — hızlı", Description: "M2.1'in hızlı varyantı (~100 tps)"},
			{ID: "MiniMax-M2", Label: "MiniMax M2 — ajan/akıl yürütme", Description: "Ajan yetenekleri + gelişmiş akıl yürütme"},
		},
	}
}

func (minimaxKind) Available(cfg ResolvedConfig) bool { return cfg.Key != "" }

func (minimaxKind) Build(cfg ResolvedConfig) (Provider, error) {
	if cfg.Key == "" {
		return nil, fmt.Errorf("minimax provider not configured (set an API key in Settings)")
	}
	return NewMinimax(cfg.Key, cfg.BaseURL), nil
}

func init() { RegisterKind(minimaxKind{}) }
