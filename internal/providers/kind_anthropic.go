package providers

import "fmt"

// anthropicKind is the Anthropic Messages API transport (native tool-use,
// prompt caching, extended thinking, token streaming).
func init() {
	RegisterKind(NewBuiltinKind(
		Manifest{
			Kind:             "anthropic",
			Label:            "Anthropic API",
			NeedsKey:         true,
			NeedsBaseURL:     true,
			AllowCustomModel: true,
			Order:            1,
			Models: []ModelInfo{
				{ID: "claude-fable-5", Label: "Claude Fable 5 — öncü", Description: "En yeni nesil; 1M bağlam, adaptif düşünme (daima açık), ajan görevleri. Not: 30 günlük veri saklama gerektirir (ZDR organizasyonlarda çalışmaz); güvenlik sınıflandırıcıları reddi Opus 4.8 fallback'iyle karşılanır (Ayarlar)"},
				{ID: "claude-opus-4-8", Label: "Claude Opus 4.8 — en yetenekli", Description: "Karmaşık akıl yürütme, kodlama, ajan görevleri"},
				{ID: "claude-sonnet-5", Label: "Claude Sonnet 5 — dengeli", Description: "En yeni dengeli nesil; 1M bağlam, güçlü ajan/kodlama, hız/kalite dengesi"},
				{ID: "claude-sonnet-4-6", Label: "Claude Sonnet 4.6 — önceki dengeli", Description: "Güçlü ve hızlı; önceki nesil dengeli model"},
				{ID: "claude-haiku-4-5-20251001", Label: "Claude Haiku 4.5 — hızlı/ucuz", Description: "Düşük gecikme, yüksek hacim, basit görevler"},
			},
		},
		func(cfg ResolvedConfig) bool { return cfg.Key != "" },
		func(cfg ResolvedConfig) (Provider, error) {
			if cfg.Key == "" {
				return nil, fmt.Errorf("anthropic provider not configured (set an API key in Settings)")
			}
			return NewAnthropic(cfg.Key).WithBetas(cfg.ExtendedCache, cfg.ContextEditing, cfg.ServerCompaction).WithRefusalFallback(cfg.RefusalFallback), nil
		},
	))
}
