package providers

// ModelInfo is one selectable model in the catalog.
type ModelInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
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
				{ID: "", Label: "Varsayılan"},
				{ID: "sonnet", Label: "Sonnet"},
				{ID: "opus", Label: "Opus"},
				{ID: "haiku", Label: "Haiku"},
			},
		},
		{
			ID:               "anthropic",
			Label:            "Anthropic API",
			NeedsKey:         true,
			AllowCustomModel: true,
			Models: []ModelInfo{
				{ID: "claude-opus-4-8", Label: "Claude Opus 4.8"},
				{ID: "claude-sonnet-4-6", Label: "Claude Sonnet 4.6"},
				{ID: "claude-haiku-4-5-20251001", Label: "Claude Haiku 4.5"},
				{ID: "claude-fable-5", Label: "Claude Fable 5"},
			},
		},
		{
			ID:               "minimax",
			Label:            "MiniMax (OpenAI-uyumlu)",
			NeedsKey:         true,
			AllowCustomModel: true,
			Models: []ModelInfo{
				{ID: "MiniMax-M2.1", Label: "MiniMax M2.1"},
				{ID: "MiniMax-M2.1-lightning", Label: "MiniMax M2.1 Lightning"},
				{ID: "MiniMax-M2", Label: "MiniMax M2"},
			},
		},
	}
}
