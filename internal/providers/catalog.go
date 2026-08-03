package providers

// ModelInfo is one selectable model in the catalog. Description is an optional
// one-line note shown as a hint under the picker. ContextWindow is the model's
// approximate context size in tokens (0 = unknown); it is filled at Catalog()
// build time from ContextWindowFor, so manifests stay free of churning numbers.
type ModelInfo struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Description   string `json:"description,omitempty"`
	ContextWindow int    `json:"contextWindow,omitempty"`
	// MaxOutput is the model's generation cap in tokens (0 = unknown). Like
	// ContextWindow it is filled at Catalog() build time from MaxOutputFor, so
	// manifests stay free of churning numbers; a manifest may still set an
	// explicit value to override the family default for a specific model.
	MaxOutput int `json:"maxOutput,omitempty"`
	// ResolvedModel is the concrete model id this entry's (possibly alias) ID was
	// last OBSERVED to mean — "opus" → "claude-opus-5". Empty until a completed
	// turn has revealed it. Filled by the API layer from the workspace's
	// observation store, never here: which model an alias points at is a runtime
	// fact that changes without a release, so a manifest must not claim it.
	ResolvedModel string `json:"resolvedModel,omitempty"`
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

// Catalog returns the provider/model catalog, derived from the registered
// provider kinds (each kind self-describes via its Manifest). Availability
// (whether a provider is actually configured) is layered on by the API handler.
// The model lists are curated suggestions — model IDs evolve, so every provider
// allows a custom value too.
func Catalog() []CatalogEntry {
	kinds := Kinds()
	out := make([]CatalogEntry, 0, len(kinds))
	for _, k := range kinds {
		m := k.Manifest()
		// Fill each model's context window from the central family table (unless a
		// manifest set an explicit value). Keeps churning numbers out of manifests.
		models := make([]ModelInfo, len(m.Models))
		copy(models, m.Models)
		for i := range models {
			if models[i].ContextWindow == 0 {
				models[i].ContextWindow = ContextWindowFor(m.Kind, models[i].ID)
			}
			if models[i].MaxOutput == 0 {
				models[i].MaxOutput = MaxOutputFor(m.Kind, models[i].ID)
			}
		}
		out = append(out, CatalogEntry{
			ID:               m.Kind,
			Label:            m.Label,
			NeedsKey:         m.NeedsKey,
			AllowCustomModel: m.AllowCustomModel,
			Models:           models,
		})
	}
	return out
}
