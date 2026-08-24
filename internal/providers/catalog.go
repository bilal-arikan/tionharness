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
	// ThinkingTiers is the set of reasoning levels this model meaningfully
	// supports, as the stable tokens the UI pickers use ("off"/"low"/"medium"/
	// "high"/"xhigh"/"max"). Filled at Catalog() build time from ThinkingTiersFor
	// so the composer/agent pickers can grey out tiers that would be a no-op on
	// the selected model (e.g. "off" on the always-on Fable class, or "xhigh"/
	// "max" on legacy models that clamp them down). Never set in manifests.
	ThinkingTiers []string `json:"thinkingTiers,omitempty"`
	// ThinkingClass is how the model handles extended reasoning (ThinkingClass):
	// "always-on"/"adaptive"/"non-thinking"/"legacy"/"alias". The pickers pair it
	// with ThinkingTiers to explain why a greyed-out tier is inactive. Filled at
	// Catalog() build time; never set in manifests.
	ThinkingClass string `json:"thinkingClass,omitempty"`
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
	// AppliesToolHooks mirrors Manifest.AppliesToolHooks — whether TionHarness's
	// PreToolUse/PostToolUse hooks (and hook-derived behaviour like sqz) fire
	// for this kind's turns. False only for codex-cli today.
	AppliesToolHooks bool `json:"appliesToolHooks"`
}

// enrichModels returns a copy of models with the family-derived metadata filled
// in from the central tables, keyed by the provider KIND (a custom instance id
// like "PRV1" would resolve nothing). Fields already set by a manifest are left
// alone, so a manifest can still override the family default for one model.
// Shared by Catalog() and Registry.InstanceCatalog() so instance-derived entries
// (including user-typed custom model ids) carry the same metadata.
func enrichModels(kind string, models []ModelInfo) []ModelInfo {
	out := make([]ModelInfo, len(models))
	copy(out, models)
	for i := range out {
		if out[i].ContextWindow == 0 {
			out[i].ContextWindow = ContextWindowFor(kind, out[i].ID)
		}
		if out[i].MaxOutput == 0 {
			out[i].MaxOutput = MaxOutputFor(kind, out[i].ID)
		}
		if out[i].ThinkingTiers == nil {
			out[i].ThinkingTiers = ThinkingTiersFor(out[i].ID)
		}
		if out[i].ThinkingClass == "" {
			out[i].ThinkingClass = ThinkingClass(out[i].ID)
		}
	}
	return out
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
		models := enrichModels(m.Kind, m.Models)
		out = append(out, CatalogEntry{
			ID:               m.Kind,
			Label:            m.Label,
			NeedsKey:         m.NeedsKey,
			AllowCustomModel: m.AllowCustomModel,
			Models:           models,
			AppliesToolHooks: m.AppliesToolHooks,
		})
	}
	return out
}
