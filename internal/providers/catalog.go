package providers

// ModelInfo is one selectable model in the catalog. Description is an optional
// one-line note shown as a hint under the picker.
type ModelInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// CatalogEntry describes a provider and its known models for the UI's
// provider/model pickers. AllowCustomModel lets the user type a model not in
// the list (model IDs change frequently).
type CatalogEntry struct {
	ID               string      `json:"id"`
	Label            string      `json:"label"`
	NeedsKey         bool        `json:"needsKey"`
	AllowCustomModel bool        `json:"allowCustomModel"`
	Models           []ModelInfo `json:"models"`
}

// Catalog returns the static provider/model catalog. Availability (whether a
// provider is actually configured) is layered on by the API handler. The model
// lists are curated suggestions — model IDs evolve, so every provider allows a
// custom value too.
func Catalog() []CatalogEntry {
	return []CatalogEntry{
		{
			ID:               "claude-cli",
			Label:            "Claude CLI (abonelik · anahtarsız)",
			NeedsKey:         false,
			AllowCustomModel: true,
			Models: []ModelInfo{
				{ID: "", Label: "Varsayılan (oturum modeli)", Description: "claude oturumunun aktif modelini kullanır"},
				{ID: "opus", Label: "Opus — en güçlü", Description: "En yetenekli; en yavaş/pahalı"},
				{ID: "sonnet", Label: "Sonnet — dengeli", Description: "Hız/kalite dengesi (günlük kullanım)"},
				{ID: "haiku", Label: "Haiku — hızlı", Description: "En hızlı/ucuz; basit görevler"},
			},
		},
		{
			ID:               "anthropic",
			Label:            "Anthropic API",
			NeedsKey:         true,
			AllowCustomModel: true,
			Models: []ModelInfo{
				{ID: "claude-opus-4-8", Label: "Claude Opus 4.8 — en yetenekli", Description: "Karmaşık akıl yürütme, kodlama, ajan görevleri"},
				{ID: "claude-sonnet-4-6", Label: "Claude Sonnet 4.6 — dengeli", Description: "Güçlü ve hızlı; çoğu iş için varsayılan tercih"},
				{ID: "claude-haiku-4-5-20251001", Label: "Claude Haiku 4.5 — hızlı/ucuz", Description: "Düşük gecikme, yüksek hacim, basit görevler"},
				{ID: "claude-fable-5", Label: "Claude Fable 5 — yaratıcı", Description: "Yaratıcı yazım/anlatı odaklı"},
			},
		},
		{
			ID:               "minimax",
			Label:            "MiniMax (OpenAI-uyumlu)",
			NeedsKey:         true,
			AllowCustomModel: true,
			Models: []ModelInfo{
				{ID: "MiniMax-M2.1", Label: "MiniMax M2.1 — çok dilli kodlama", Description: "Güçlü kodlama, çok dil (~60 tps)"},
				{ID: "MiniMax-M2.1-lightning", Label: "MiniMax M2.1 Lightning — hızlı", Description: "M2.1'in hızlı varyantı (~100 tps)"},
				{ID: "MiniMax-M2", Label: "MiniMax M2 — ajan/akıl yürütme", Description: "Ajan yetenekleri + gelişmiş akıl yürütme"},
			},
		},
	}
}
