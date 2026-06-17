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
		out = append(out, CatalogEntry{
			ID:               m.Kind,
			Label:            m.Label,
			NeedsKey:         m.NeedsKey,
			AllowCustomModel: m.AllowCustomModel,
			Models:           m.Models,
		})
	}
	return out
}
