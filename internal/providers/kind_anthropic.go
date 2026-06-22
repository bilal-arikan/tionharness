package providers

import "fmt"

// anthropicKind is the Anthropic Messages API transport (native tool-use,
// prompt caching, extended thinking, token streaming).
type anthropicKind struct{}

func (anthropicKind) Manifest() Manifest {
	return Manifest{
		Kind:             "anthropic",
		Label:            "Anthropic API",
		NeedsKey:         true,
		NeedsBaseURL:     true,
		AllowCustomModel: true,
		Order:            1,
		Models: []ModelInfo{
			{ID: "claude-opus-4-8", Label: "Claude Opus 4.8 — en yetenekli", Description: "Karmaşık akıl yürütme, kodlama, ajan görevleri"},
			{ID: "claude-sonnet-4-6", Label: "Claude Sonnet 4.6 — dengeli", Description: "Güçlü ve hızlı; çoğu iş için varsayılan tercih"},
			{ID: "claude-haiku-4-5-20251001", Label: "Claude Haiku 4.5 — hızlı/ucuz", Description: "Düşük gecikme, yüksek hacim, basit görevler"},
			{ID: "claude-fable-5", Label: "Claude Fable 5 — yaratıcı", Description: "Yaratıcı yazım/anlatı odaklı"},
		},
	}
}

func (anthropicKind) Available(cfg ResolvedConfig) bool { return cfg.Key != "" }

func (anthropicKind) Build(cfg ResolvedConfig) (Provider, error) {
	if cfg.Key == "" {
		return nil, fmt.Errorf("anthropic provider not configured (set an API key in Settings)")
	}
	return NewAnthropic(cfg.Key).WithBetas(cfg.ExtendedCache), nil
}

func init() { RegisterKind(anthropicKind{}) }
