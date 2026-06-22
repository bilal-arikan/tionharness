package providers

import "sort"

// Manifest is a provider kind's self-description. It is the seam that makes
// providers behave like plugins: the registry, the catalog and (later) the
// settings UI all consume a kind purely through its Manifest, never through
// kind-specific code. Adding a transport means adding a ProviderKind that
// registers itself — nothing else in the system has to change.
type Manifest struct {
	// Kind is the stable identifier used everywhere a provider is referenced
	// (Agent.Provider, settings.DefaultProvider, catalog ID). For the built-in
	// trio these are "anthropic", "claude-cli" and "minimax".
	Kind string
	// Label is the human-facing name shown in pickers.
	Label string
	// NeedsKey reports whether the kind requires an API key (UI shows a key
	// field; availability depends on the key being set).
	NeedsKey bool
	// NeedsBaseURL reports whether the kind accepts a custom base URL (HTTP
	// transports); false for the CLI transport. Not yet surfaced to the API —
	// reserved for the data-driven instance UI (Faz 2).
	NeedsBaseURL bool
	// AllowCustomModel lets the user type a model not in Models (model IDs
	// change frequently).
	AllowCustomModel bool
	// Order controls the catalog sort position (lower first).
	Order int
	// Models is the curated suggestion list for the picker.
	Models []ModelInfo
}

// ResolvedConfig carries the credentials and options a kind needs to build a
// concrete client. The Registry fills it from its live (mutable) fields per
// provider id. Fields a given kind does not use are simply left zero — the CLI
// transport ignores Key/BaseURL, the HTTP transports ignore CLIPath.
type ResolvedConfig struct {
	Key           string // API key (anthropic, minimax)
	BaseURL       string // custom endpoint ("" = kind default)
	Model         string // default model applied when a request omits one
	CLIPath       string // resolved `claude` binary path (claude-cli)
	ExtendedCache bool   // anthropic extended prompt-cache beta
}

// ProviderKind is one transport "plugin": it describes itself (Manifest),
// reports whether a given configuration makes it usable (Available) and builds
// a concrete Provider (Build). The three built-ins live in kind_*.go and
// register themselves via init(); a fourth transport is a new file plus a
// RegisterKind call, with no edits to the registry or catalog.
type ProviderKind interface {
	Manifest() Manifest
	Available(cfg ResolvedConfig) bool
	Build(cfg ResolvedConfig) (Provider, error)
}

// kindRegistry holds the registered transports. It is package-global and
// written only from init() (single-threaded at load), then read-only — so no
// lock is needed.
var kindRegistry = map[string]ProviderKind{}

// RegisterKind adds a transport to the registry, keyed by its Manifest().Kind.
// A later registration for the same kind overrides the earlier one.
func RegisterKind(k ProviderKind) {
	kindRegistry[k.Manifest().Kind] = k
}

// lookupKind returns the kind for an id (normalizing "" to the keyless CLI
// default, matching the historical Registry.Get behaviour).
func lookupKind(id string) (ProviderKind, bool) {
	if id == "" {
		id = "claude-cli"
	}
	k, ok := kindRegistry[id]
	return k, ok
}

// Kinds returns every registered kind sorted by Manifest().Order, giving a
// stable catalog ordering independent of init() evaluation order.
func Kinds() []ProviderKind {
	out := make([]ProviderKind, 0, len(kindRegistry))
	for _, k := range kindRegistry {
		out = append(out, k)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Manifest().Order < out[j].Manifest().Order
	})
	return out
}
