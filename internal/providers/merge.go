package providers

// MergeCatalog combines the built-in provider catalog with user-added custom
// providers, letting each custom entry OVERRIDE any built-in that shares its ID
// rather than appending a second entry with the same ID. A duplicate provider ID
// is ambiguous (which one does an agent's `provider: "openrouter"` mean?) and, in
// the UI, produces a React duplicate-key warning in the provider pickers.
//
// Precedence is "user config wins": a custom provider is explicitly configured
// (its own key / base URL / model list), so when its ID matches a built-in the
// custom entry replaces the built-in in place. Custom IDs with no built-in match
// are appended after the built-ins, preserving order.
//
// The input slices are not mutated; the result is a fresh slice.
func MergeCatalog(builtin, custom []CatalogEntry) []CatalogEntry {
	out := make([]CatalogEntry, len(builtin))
	copy(out, builtin)
	byID := make(map[string]int, len(out))
	for i, e := range out {
		byID[e.ID] = i
	}
	for _, c := range custom {
		if i, ok := byID[c.ID]; ok {
			out[i] = c
		} else {
			byID[c.ID] = len(out)
			out = append(out, c)
		}
	}
	return out
}
