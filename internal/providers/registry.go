package providers

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
)

// Instance is the registry's view of one provider instance (Faz 2,
// _Docs/71 §2.3): a kind id plus its resolved (decrypted) config/secret
// values, keyed by the kind's declared FieldSpec.Key. It is a package-local
// mirror of settings.ProviderInstance rather than a direct dependency on it,
// keeping internal/providers free of an internal/settings import — the
// caller (internal/api) projects settings.ProviderInstance + its decrypted
// secrets into this shape via ToRegistryInstance.
type Instance struct {
	ID           string
	KindID       string
	Label        string
	Enabled      bool
	DefaultModel string
	Models       string
	// Values holds every field the instance's kind declares (Manifest.Fields),
	// both open config and decrypted secrets, keyed by FieldSpec.Key.
	Values map[string]string
	// Reasoning/PromptCache are the openai-compat-only capability flags carried
	// over from the legacy CustomSpec (_Docs/71 §3's openai-compat migration);
	// not a standard FieldSpec key because no built-in kind uses them.
	Reasoning   bool
	PromptCache string
}

// Registry builds providers by instance id, resolving each instance's config
// through its registered kind. Its fields are mutable at runtime so the
// Settings screen can update instances or CLI paths live.
type Registry struct {
	mu sync.RWMutex

	claudeCLIPath string // autodetected `claude` binary path fallback, or "" if absent
	codexCLIPath  string // autodetected `codex` binary path fallback, or "" if absent

	betaExtendedCache    bool // anthropic extended prompt-cache TTL beta
	betaContextEditing   bool // anthropic API-native context-editing beta (clear_tool_uses)
	betaServerCompaction bool // anthropic API-native compaction beta (compact_20260112)
	betaRefusalFallback  bool // anthropic server-side refusal fallback (Fable-class requests)

	instances map[string]Instance // provider instances, keyed by ID (Faz 2, _Docs/71 §2.3)
}

// NewRegistry creates a registry. It auto-detects the keyless CLI transports
// (claude, codex) on PATH so they work out of the box as the fallback binary
// path when an instance leaves its own cliPath field empty.
func NewRegistry() *Registry {
	claudePath, _ := exec.LookPath("claude")
	codexPath := lookupCodexBinary()
	return &Registry{
		claudeCLIPath: claudePath,
		codexCLIPath:  codexPath,
		instances:     map[string]Instance{},
	}
}

// SetInstances replaces the full set of provider instances the registry
// resolves against (Faz 2, _Docs/71 §2.3/§4.3). Called from applySettings
// whenever settings or providers.json change. Unlike the old per-kind Set*
// methods, there is exactly one entry point regardless of how many kinds or
// instances exist.
func (r *Registry) SetInstances(list []Instance) {
	m := make(map[string]Instance, len(list))
	for _, inst := range list {
		if inst.ID == "" {
			continue
		}
		m[inst.ID] = inst
	}
	r.mu.Lock()
	r.instances = m
	r.mu.Unlock()
}

// SetClaudeCLIPath overrides the autodetected claude binary path fallback used
// when an instance's own cliPath field is empty. An empty value re-runs PATH
// auto-detection so clearing the override restores default behaviour.
func (r *Registry) SetClaudeCLIPath(path string) {
	if path == "" {
		path, _ = exec.LookPath("claude")
	}
	r.mu.Lock()
	r.claudeCLIPath = path
	r.mu.Unlock()
}

// SetCodexCLIPath overrides the autodetected codex binary path fallback used
// when an instance's own cliPath field is empty. An empty value re-runs PATH
// auto-detection so clearing the override restores default behaviour.
func (r *Registry) SetCodexCLIPath(path string) {
	if path == "" {
		path = lookupCodexBinary()
	}
	r.mu.Lock()
	r.codexCLIPath = path
	r.mu.Unlock()
}

// SetAnthropicBetas toggles the optional Anthropic beta capabilities applied to
// anthropic provider instances. These stay app-wide (not per-instance): they
// are experimental API behaviour switches, not credentials.
func (r *Registry) SetAnthropicBetas(extendedCache, contextEditing, serverCompaction, refusalFallback bool) {
	r.mu.Lock()
	r.betaExtendedCache = extendedCache
	r.betaContextEditing = contextEditing
	r.betaServerCompaction = serverCompaction
	r.betaRefusalFallback = refusalFallback
	r.mu.Unlock()
}

// InstanceSummary is the credential-free view of one provider instance: the
// fields an agent needs to PICK an instance (id, kind, label, whether it is
// enabled, its models) and nothing that could leak a key. Instance.Values —
// which holds decrypted secrets — is deliberately absent.
type InstanceSummary struct {
	ID           string `json:"id"`
	KindID       string `json:"kindId"`
	Label        string `json:"label"`
	Enabled      bool   `json:"enabled"`
	DefaultModel string `json:"defaultModel,omitempty"`
	Models       string `json:"models,omitempty"`
	// Available mirrors Registry.Available: whether the instance is actually
	// usable right now (kind registered, required credentials/binaries present).
	Available bool `json:"available"`
}

// ListInstances returns every configured provider instance — enabled or not —
// as a credential-free summary, sorted by id for a stable listing. Unlike
// InstanceCatalog it does not drop disabled instances: a caller listing
// providers needs to see that an instance exists but is switched off.
func (r *Registry) ListInstances() []InstanceSummary {
	r.mu.RLock()
	out := make([]InstanceSummary, 0, len(r.instances))
	for _, inst := range r.instances {
		out = append(out, InstanceSummary{
			ID:           inst.ID,
			KindID:       inst.KindID,
			Label:        inst.Label,
			Enabled:      inst.Enabled,
			DefaultModel: inst.DefaultModel,
			Models:       inst.Models,
		})
	}
	r.mu.RUnlock()
	// Available takes the lock itself, so it is resolved outside the section above.
	for i := range out {
		out[i].Available = r.Available(out[i].ID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// InstanceCatalog returns one catalog entry per configured, enabled provider
// instance, keyed by the INSTANCE id rather than its kind — so two instances
// of the same kind (e.g. two Anthropic accounts) each get their own catalog
// entry instead of collapsing into Catalog()'s single per-kind entry
// (_Docs/71 Faz 5 item 3). Each entry inherits its kind's manifest metadata
// (NeedsKey/AllowCustomModel/AppliesToolHooks/curated Models), with the
// instance's own Models override taking precedence when set — mirroring
// resolveInstance's kind lookup. An instance whose kind is no longer
// registered is skipped rather than erroring: the catalog is a best-effort
// picker aid, not the source of truth Registry.Get enforces.
//
// MergeCatalog (caller-applied) lets a same-ID instance entry override
// Catalog()'s per-kind entry in place — the default migrated instance's ID
// equals its kind ID (_Docs/71 §3), so a single, unconfigured-by-hand
// "anthropic" instance still shows once, not twice.
func (r *Registry) InstanceCatalog() []CatalogEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CatalogEntry, 0, len(r.instances))
	for _, inst := range r.instances {
		if !inst.Enabled {
			continue
		}
		k, ok := lookupKind(inst.KindID)
		if !ok {
			continue
		}
		m := k.Manifest()
		models := parseModelList(inst.Models)
		if len(models) == 0 {
			models = m.Models
		}
		// Enrich by KIND, not by inst.ID: a custom instance id ("PRV1") resolves no
		// family metadata, and custom model lists must be enriched too.
		models = enrichModels(inst.KindID, models)
		out = append(out, CatalogEntry{
			ID:               inst.ID,
			Label:            inst.Label,
			NeedsKey:         m.NeedsKey,
			AllowCustomModel: m.AllowCustomModel,
			Models:           models,
			AppliesToolHooks: m.AppliesToolHooks,
		})
	}
	return out
}

// parseModelList turns a comma/newline separated id list into ModelInfo entries
// (label = id). Blank entries are skipped.
func parseModelList(s string) []ModelInfo {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
	out := make([]ModelInfo, 0, len(fields))
	for _, f := range fields {
		id := strings.TrimSpace(f)
		if id != "" {
			out = append(out, ModelInfo{ID: id, Label: id})
		}
	}
	return out
}

// ClaudeCLIAvailable reports whether the claude CLI was found.
func (r *Registry) ClaudeCLIAvailable() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.claudeCLIPath != ""
}

// ClaudeCLIPath returns the resolved path to the `claude` binary, or "" when it
// was not found. Callers use it to probe the local install (version, login) —
// building a provider is not needed just to ask about the binary.
func (r *Registry) ClaudeCLIPath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.claudeCLIPath
}

// CodexCLIAvailable reports whether the codex CLI was found.
func (r *Registry) CodexCLIAvailable() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.codexCLIPath != ""
}

// CodexCLIPath returns the resolved path to the `codex` binary, or "" when it
// was not found. Callers use it to probe the local install (version, login) —
// building a provider is not needed just to ask about the binary.
func (r *Registry) CodexCLIPath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.codexCLIPath
}

// resolveInstance returns the registered instance for id and its kind (looked
// up by the instance's KindID), or an error if either is missing. The empty
// id maps to the keyless "claude-cli" default instance, matching the
// historical Registry.Get behaviour for an unset Agent.Provider. Unlike the
// pre-Faz-2 switch-on-id seam, an unknown/deleted instance is a hard error —
// there is no silent claude-cli fallback for a real (non-empty) id
// (_Docs/71 §4.3, K3 risk table: "silent orphaned agent").
func (r *Registry) resolveInstance(id string) (Instance, ProviderKind, error) {
	if id == "" {
		id = "claude-cli"
	}
	r.mu.RLock()
	inst, ok := r.instances[id]
	r.mu.RUnlock()
	if !ok {
		return Instance{}, nil, fmt.Errorf("unknown provider instance: %q", id)
	}
	k, ok := lookupKind(inst.KindID)
	if !ok {
		return Instance{}, nil, fmt.Errorf("provider instance %q has unregistered kind %q", id, inst.KindID)
	}
	return inst, k, nil
}

// KindOf returns the kind id of the provider instance identified by id, or ""
// if the instance is not registered. Used to keep Agent.Provider synchronised
// with Agent.ProviderInstanceID on every agent write (_Docs/71 §2.5, K3) and by
// the API/UI to badge an instance with its kind — never by billing/context
// callers, which key off Agent.Provider directly and must never see a raw
// instance id (_Docs/71 §4.1).
func (r *Registry) KindOf(id string) string {
	if id == "" {
		id = "claude-cli"
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.instances[id].KindID
}

// resolve assembles the ResolvedConfig a kind needs from a resolved instance's
// values plus the registry's app-wide fields (autodetected CLI path fallback,
// Anthropic betas). Standard field keys (FieldKeyAPIKey, FieldKeyBaseURL, ...)
// map onto ResolvedConfig's typed fields so every existing kind_*.go Build
// function keeps working unchanged; Values carries the full raw map for kinds
// that need more (openai-compat, anthropic-compat).
func (r *Registry) resolve(inst Instance) ResolvedConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := inst.Values
	cliPath := v[FieldKeyCLIPath]
	if cliPath == "" {
		cliPath = r.claudeCLIPath
	}
	codexPath := v[FieldKeyCLIPath]
	if codexPath == "" {
		codexPath = r.codexCLIPath
	}
	return ResolvedConfig{
		Key:              v[FieldKeyAPIKey],
		BaseURL:          v[FieldKeyBaseURL],
		CLIPath:          cliPath,
		CLIConfigDir:     v[FieldKeyConfigDir],
		CLIAuthKind:      v[FieldKeyAuthKind],
		CLIAuthToken:     v[FieldKeyAuthToken],
		CodexPath:        codexPath,
		CodexConfigDir:   v[FieldKeyConfigDir],
		ExtendedCache:    r.betaExtendedCache,
		ContextEditing:   r.betaContextEditing,
		ServerCompaction: r.betaServerCompaction,
		RefusalFallback:  r.betaRefusalFallback,
		InstanceID:       inst.ID,
		DefaultModel:     inst.DefaultModel,
		Models:           inst.Models,
		Reasoning:        inst.Reasoning,
		PromptCache:      inst.PromptCache,
		Values:           v,
	}
}

// Available reports whether the provider instance id is registered and usable
// with its current configuration (key set / CLI present). Used by the catalog
// handler. An unregistered id reports false rather than erroring — the
// catalog only needs a yes/no badge.
// FirstAvailableOfKind returns the id of the first ENABLED, available provider
// instance of the given kind (ids sorted, so the pick is deterministic), or ""
// when none is usable. Auxiliary-call routing uses it to find a configured
// first-party API instance without hard-coding the instance id.
func (r *Registry) FirstAvailableOfKind(kind string) string {
	r.mu.RLock()
	var ids []string
	for _, inst := range r.instances {
		if inst.KindID == kind && inst.Enabled {
			ids = append(ids, inst.ID)
		}
	}
	r.mu.RUnlock()
	sort.Strings(ids)
	for _, id := range ids {
		if r.Available(id) {
			return id
		}
	}
	return ""
}

// Reachable probes a local instance's server and returns the verdict, or nil
// when the instance is not a local endpoint (every hosted kind). Probes are
// cached, so listing providers repeatedly costs one request per server per few
// seconds rather than one per call.
//
// It is separate from Available on purpose: Available is a synchronous, pure
// configuration check called on every catalog build and must never do I/O,
// while a local instance can be fully configured (Available) yet unusable
// because the app behind it is closed.
func (r *Registry) Reachable(id string) *bool {
	inst, k, err := r.resolveInstance(id)
	if err != nil {
		return nil
	}
	got, ok := LocalEndpointReachable(context.Background(), k.Manifest().Kind, r.resolve(inst))
	if !ok {
		return nil
	}
	return &got
}

func (r *Registry) Available(id string) bool {
	inst, k, err := r.resolveInstance(id)
	if err != nil {
		return false
	}
	return k.Available(r.resolve(inst))
}

// Get returns a provider for the given provider INSTANCE id, or an error if
// the instance is unregistered or its kind is unconfigured. The empty id maps
// to the keyless claude-cli default instance.
func (r *Registry) Get(id string) (Provider, error) {
	inst, k, err := r.resolveInstance(id)
	if err != nil {
		return nil, err
	}
	prov, err := k.Build(r.resolve(inst))
	if err != nil {
		return nil, err
	}
	// Apply the kind's declared per-request budget override (0 = leave the
	// model-class default). Threading it here keeps every kind's build function
	// free of timeout plumbing; clients that support it implement the interface.
	if secs := k.Manifest().RequestTimeoutSecs; secs > 0 {
		if tc, ok := prov.(requestTimeoutConfigurable); ok {
			tc.setRequestTimeout(secs)
		}
	}
	return prov, nil
}
