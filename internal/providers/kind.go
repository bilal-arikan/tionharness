package providers

import "sort"

// Standard field keys. Kinds declare their instance form using these keys so
// that a future ResolvedConfig (Faz 2) can map them 1:1 onto its typed fields
// (Key, BaseURL, CLIPath, CLIConfigDir, CLIAuthKind, CLIAuthToken) without a
// per-kind translation layer.
const (
	FieldKeyAPIKey    = "key"
	FieldKeyBaseURL   = "baseUrl"
	FieldKeyCLIPath   = "cliPath"
	FieldKeyConfigDir = "configDir"
	FieldKeyAuthKind  = "authKind"
	FieldKeyAuthToken = "authToken"
)

// Transport identifies how a kind's Provider talks to the model: a direct API
// call (native tool loop drives the request) or a local CLI subprocess (the
// CLI runs its own agentic loop). See Manifest.Transport.
const (
	TransportAPI = "api"
	TransportCLI = "cli"
)

// FieldSpec describes one field of a provider kind's instance form. A kind
// declares its whole settings form as []FieldSpec instead of the settings UI
// and API DTO hard-coding a bespoke form per kind (Faz 2/3 consume this to
// render/validate generically).
type FieldSpec struct {
	// Key is the field's identifier within the kind's Config/Secrets map. Use
	// the FieldKey* constants for the standard fields; a kind may also declare
	// a kind-specific key not in that list.
	Key string
	// Label is the human-facing field name shown in the settings form.
	Label string
	// Type selects the form control and validation: "text" | "password" |
	// "path" | "dir" | "select".
	Type string
	// Options is the choice list for Type=="select"; unused otherwise.
	Options []string
	// Required reports whether an instance must supply a non-empty value.
	Required bool
	// Default is the value assumed when the field is left empty.
	Default string
	// Placeholder is example/hint text shown in the empty field.
	Placeholder string
	// Help is longer explanatory text shown below the field.
	Help string
	// Secret reports whether the value is stored encrypted (AES-GCM) and
	// masked in API responses. Every Secret field must use Type=="password".
	Secret bool
}

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
	// AppliesToolHooks reports whether TionHarness's PreToolUse/PostToolUse hooks
	// (and hook-derived behaviour like sqz/PostToolUse token-optimizer
	// compression) actually fire for this kind's turns. True for the native
	// tool loop and for claude-cli (which forwards hooks via --settings).
	// False for a CLI transport that runs its own agentic loop in a subprocess
	// with no hook passthrough of its own (codex-cli) — for those, neither the
	// CLI's native tools nor TionHarness tools reached over the MCP bridge run
	// hooks. Surfaced to the UI via the catalog so this silent gap is visible
	// instead of assumed.
	AppliesToolHooks bool
	// RequestTimeoutSecs overrides the per-request wall-clock budget (seconds) for
	// every client this kind builds. 0 = model-class default (120s, or the long
	// budget for reasoning/adaptive models — see LongRequestModel). Lets a slow
	// endpoint declare a longer budget declaratively; Registry.Get applies it to
	// the built provider after Build, with no per-kind build-function change.
	RequestTimeoutSecs int
	// Models is the curated suggestion list for the picker.
	Models []ModelInfo
	// Transport is the behavioural switch consumed by call sites that today
	// string-compare against "claude-cli" (skill-tool naming, lazy tool
	// catalog form, hook-passthrough UI badges — see Faz 2 §4.2). "api" for a
	// direct HTTP transport, "cli" for a local subprocess running its own
	// agentic loop. Not yet consumed anywhere in this phase — declared only.
	Transport string
	// Fields is the kind's whole instance settings form, replacing the
	// per-kind hard-coded forms in settings.go/ProvidersPanel.tsx (Faz 2/3).
	Fields []FieldSpec
	// Multi reports whether more than one instance of this kind is meaningful
	// (e.g. two Anthropic API keys for two accounts). Not yet enforced.
	Multi bool
	// TemplateOnly marks a kind that exists purely so a user CAN create an
	// instance from it (openai-compat, anthropic-compat) rather than one a
	// user picks directly — it ships with no default/migrated instance of its
	// own. Catalog() still lists it (so /api/provider-kinds can offer it as a
	// template to build a custom provider from), but the model-picker catalog
	// (/api/catalog, providers.Catalog()+MergeCatalog) drops its per-kind
	// entry when no instance of it exists, instead of showing a permanently
	// unavailable, model-less placeholder.
	TemplateOnly bool
}

// FieldByKey returns the field spec with the given Key, if the manifest
// declares one.
func (m Manifest) FieldByKey(key string) (FieldSpec, bool) {
	for _, f := range m.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return FieldSpec{}, false
}

// TransportOf returns the registered kind's Manifest.Transport, or "" if
// kindID is not a registered kind.
func TransportOf(kindID string) string {
	k, ok := kindRegistry[kindID]
	if !ok {
		return ""
	}
	return k.Manifest().Transport
}

// ResolvedConfig carries the credentials and options a kind needs to build a
// concrete client. The Registry fills it from its live (mutable) fields per
// provider id. Fields a given kind does not use are simply left zero — the CLI
// transport ignores Key/BaseURL, the HTTP transports ignore CLIPath.
type ResolvedConfig struct {
	Key              string // API key (anthropic, minimax)
	BaseURL          string // custom endpoint ("" = kind default)
	CLIPath          string // resolved `claude` binary path (claude-cli)
	CLIConfigDir     string // CLAUDE_CONFIG_DIR override for the claude-cli subprocess ("" = inherit ~/.claude)
	CLIAuthKind      string // claude-cli credential kind: "oauth" | "apikey" | "" (none)
	CLIAuthToken     string // claude-cli credential value injected into the subprocess env
	CodexPath        string // resolved `codex` binary path (codex-cli)
	CodexConfigDir   string // CODEX_HOME override for the codex-cli subprocess ("" = inherit ~/.codex)
	ExtendedCache    bool   // anthropic extended prompt-cache beta
	ContextEditing   bool   // anthropic API-native context-editing beta (clear_tool_uses)
	ServerCompaction bool   // anthropic API-native compaction beta (compact_20260112)
	RefusalFallback  bool   // anthropic server-side refusal fallback (Fable-class requests)

	// InstanceID/DefaultModel/Models/Values carry per-INSTANCE data for the
	// generic openai-compat/anthropic-compat kinds (Faz 2, _Docs/71 §3): unlike
	// every other field above (which mirrors an app-wide Registry setting),
	// these vary per instance of the SAME kind (two "openai-compat" instances
	// have different base URLs, models and reasoning/cache capability flags).
	InstanceID   string            // the resolved provider instance's id (client "name")
	DefaultModel string            // instance's configured default model, if any
	Models       string            // instance's model-id suggestion list (comma/newline)
	Reasoning    bool              // send reasoning_effort (openai-compat only)
	PromptCache  string            // "native" | "auto" | "none" | "" (openai-compat only)
	Values       map[string]string // raw resolved field values, keyed by FieldSpec.Key
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

// basicKind is the data-driven ProviderKind used by every built-in transport.
// A kind is fully described by three values — its static Manifest, an Available
// predicate over the resolved config, and a Build function — so each kind_*.go
// file is just those three values wired through NewBuiltinKind, with no bespoke
// struct type or method set to repeat. A transport that needs behaviour beyond
// these three (none today) can still implement ProviderKind directly.
type basicKind struct {
	manifest  Manifest
	available func(ResolvedConfig) bool
	build     func(ResolvedConfig) (Provider, error)
}

func (k basicKind) Manifest() Manifest                { return k.manifest }
func (k basicKind) Available(cfg ResolvedConfig) bool { return k.available(cfg) }
func (k basicKind) Build(cfg ResolvedConfig) (Provider, error) {
	return k.build(cfg)
}

// NewBuiltinKind assembles a ProviderKind from its manifest, availability
// predicate and build function. It is the one-liner every kind_*.go registers
// through, collapsing the former per-kind struct + three method declarations
// into a single self-registering init() call.
func NewBuiltinKind(m Manifest, available func(ResolvedConfig) bool, build func(ResolvedConfig) (Provider, error)) ProviderKind {
	return basicKind{manifest: m, available: available, build: build}
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
