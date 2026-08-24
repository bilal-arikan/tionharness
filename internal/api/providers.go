package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/settings"
)

// registryInstances projects every persisted provider instance (providers.json,
// _Docs/71 §2.4) into the registry's Instance view, decrypting each instance's
// secrets through the SAME ProviderStore that owns them. Used by applySettings
// to push the full instance set into the registry on every settings change —
// the Faz 2 successor to customProviderSpecs (_Docs/71 §4.3).
func (s *Server) registryInstances() []providers.Instance {
	list := s.providerStore.List()
	out := make([]providers.Instance, 0, len(list))
	for _, inst := range list {
		values := make(map[string]string, len(inst.Config)+len(inst.SecretsEnc))
		for k, v := range inst.Config {
			values[k] = v
		}
		for k := range inst.SecretsEnc {
			values[k] = s.providerStore.Secret(inst.ID, k)
		}
		out = append(out, providers.Instance{
			ID:           inst.ID,
			KindID:       inst.KindID,
			Label:        inst.Label,
			Enabled:      inst.Enabled,
			DefaultModel: inst.DefaultModel,
			Models:       inst.Models,
			Values:       values,
			// Reasoning/PromptCache carry the legacy CustomProvider capability flags
			// forward for a migrated openai-compat instance (_Docs/71 §3); config
			// keys "reasoning"/"promptCache" are not standard FieldSpec keys so they
			// are read directly off Config rather than through Values.
			Reasoning:   inst.Config["reasoning"] == "true",
			PromptCache: inst.Config["promptCache"],
		})
	}
	return out
}

// instanceFieldValue returns the value of one FieldSpec key on the instance
// with the given id, or "" if the instance is absent or the key is unset. Used
// to keep the external-tools panel's binary-path override pointed at whatever
// the default claude-cli/codex-cli instance actually resolves to (server.go
// applySettings).
func instanceFieldValue(instances []providers.Instance, id, key string) string {
	for _, inst := range instances {
		if inst.ID == id {
			return inst.Values[key]
		}
	}
	return ""
}

// providerFieldSpecDTO is the client-facing projection of providers.FieldSpec —
// identical shape, just JSON-tagged (the internal type deliberately has none,
// since it is consumed in-process elsewhere too).
type providerFieldSpecDTO struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
	Secret      bool     `json:"secret"`
}

// providerKindDTO describes one registered provider kind's instance form — the
// data the settings UI needs to render a generic "add provider" form without
// any kind hard-coded on the frontend (_Docs/71 §2.2/§5).
type providerKindDTO struct {
	ID        string                 `json:"id"`
	Label     string                 `json:"label"`
	Transport string                 `json:"transport"`
	Multi     bool                   `json:"multi"`
	Fields    []providerFieldSpecDTO `json:"fields"`
	Models    []providers.ModelInfo  `json:"models,omitempty"`
}

// handleListProviderKinds returns the catalog of registered provider kinds
// (taslak), each with its full instance form derived from its Manifest. Built
// by walking providers.Kinds() — no kind list is hard-coded here (_Docs/71
// Faz 3 item 2).
func (s *Server) handleListProviderKinds(w http.ResponseWriter, _ *http.Request) {
	kinds := providers.Kinds()
	out := make([]providerKindDTO, 0, len(kinds))
	for _, k := range kinds {
		m := k.Manifest()
		fields := make([]providerFieldSpecDTO, 0, len(m.Fields))
		for _, f := range m.Fields {
			fields = append(fields, providerFieldSpecDTO{
				Key:         f.Key,
				Label:       f.Label,
				Type:        f.Type,
				Options:     f.Options,
				Required:    f.Required,
				Default:     f.Default,
				Placeholder: f.Placeholder,
				Help:        f.Help,
				Secret:      f.Secret,
			})
		}
		out = append(out, providerKindDTO{
			ID:        m.Kind,
			Label:     m.Label,
			Transport: m.Transport,
			Multi:     m.Multi,
			Fields:    fields,
			Models:    m.Models,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListProviders returns every configured provider instance (secrets
// masked to a per-key boolean, never plaintext or ciphertext).
func (s *Server) handleListProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.providerStore.DTOs())
}

// handleGetProvider returns one provider instance by id.
func (s *Server) handleGetProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, ok := s.providerStore.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "provider instance not found")
		return
	}
	writeJSON(w, http.StatusOK, inst.ToDTO())
}

// upsertProviderInstanceReq mirrors settings.ProviderInstanceInput; kept as a
// separate JSON-facing type (rather than binding directly into the settings
// type) for the same reason every other handler in this package does — the
// wire shape and the storage shape are allowed to diverge without a silent
// coupling. Today they are identical field-for-field.
type upsertProviderInstanceReq struct {
	ID           string            `json:"id"`
	KindID       string            `json:"kindId"`
	Label        string            `json:"label"`
	Icon         string            `json:"icon"`
	Enabled      bool              `json:"enabled"`
	DefaultModel string            `json:"defaultModel"`
	Models       string            `json:"models"`
	Config       map[string]string `json:"config"`
	Secrets      map[string]string `json:"secrets"`
}

// upsertProviderOpts carries the non-request knobs upsertProviderInstance
// callers may need. The zero value is the strict, HTTP-handler behaviour.
type upsertProviderOpts struct {
	// ExtraConfig is merged into the stored Config AFTER validation, bypassing
	// the FieldByKey whitelist — reserved for the legacy "reasoning"/
	// "promptCache" capability flags a migrated/pack-installed openai-compat
	// instance carries (not standard FieldSpec keys; registryInstances reads
	// them back the same way MigrateFromSettings writes them,
	// internal/settings/provider_migrate.go). Callers building a request from
	// user/form input MUST leave this nil — the whitelist is the only thing
	// stopping an arbitrary client-supplied key from landing in Config.
	ExtraConfig map[string]string
	// AllowMissingRequiredSecrets skips the required-secret check (but not
	// required-plain-field or unknown-kind/-field checks). Reserved for the
	// market pack installer, which has always allowed installing a provider
	// pack without its API key — the user adds it later in Settings — and
	// that pre-existing behaviour is preserved rather than silently tightened
	// by routing pack installs through this shared path.
	AllowMissingRequiredSecrets bool
}

// upsertProviderInstance validates req against its declared kind's field
// schema and writes it through ProviderStore — the single path both the HTTP
// handler and the market pack installer use, so a pack with an unknown kind
// or a missing required plain field fails exactly like a manual form submit
// (_Docs/71 Faz 3 item 1: no separate, laxer write path).
func (s *Server) upsertProviderInstance(req upsertProviderInstanceReq, opts upsertProviderOpts) (settings.ProviderInstance, error) {
	kindID := strings.TrimSpace(req.KindID)
	if kindID == "" {
		return settings.ProviderInstance{}, httpErr{http.StatusBadRequest, "kindId is required"}
	}
	kind, known := providerKindByID(kindID)
	if !known {
		return settings.ProviderInstance{}, httpErr{http.StatusBadRequest, "unknown provider kind: " + kindID}
	}
	manifest := kind.Manifest()

	for key := range req.Config {
		if _, ok := manifest.FieldByKey(key); !ok {
			return settings.ProviderInstance{}, httpErr{http.StatusBadRequest, "unknown field \"" + key + "\" for kind " + kindID}
		}
	}
	for key := range req.Secrets {
		f, ok := manifest.FieldByKey(key)
		if !ok {
			return settings.ProviderInstance{}, httpErr{http.StatusBadRequest, "unknown field \"" + key + "\" for kind " + kindID}
		}
		if !f.Secret {
			return settings.ProviderInstance{}, httpErr{http.StatusBadRequest, "field \"" + key + "\" is not a secret field for kind " + kindID}
		}
	}
	for _, f := range manifest.Fields {
		if !f.Required {
			continue
		}
		if f.Secret {
			if opts.AllowMissingRequiredSecrets {
				continue
			}
			if v, present := req.Secrets[f.Key]; present && v != "" {
				continue
			}
			// A required secret is also satisfied by an already-stored value on
			// update (the write-only convention: omitted = keep existing).
			if req.ID != "" {
				if existing, ok := s.providerStore.Get(req.ID); ok && existing.SecretsEnc[f.Key] != "" {
					continue
				}
			}
			return settings.ProviderInstance{}, httpErr{http.StatusBadRequest, "field \"" + f.Key + "\" is required for kind " + kindID}
		}
		if strings.TrimSpace(req.Config[f.Key]) == "" && strings.TrimSpace(f.Default) == "" {
			return settings.ProviderInstance{}, httpErr{http.StatusBadRequest, "field \"" + f.Key + "\" is required for kind " + kindID}
		}
	}

	cfg := req.Config
	if len(opts.ExtraConfig) > 0 {
		cfg = make(map[string]string, len(req.Config)+len(opts.ExtraConfig))
		for k, v := range req.Config {
			cfg[k] = v
		}
		for k, v := range opts.ExtraConfig {
			cfg[k] = v
		}
	}

	inst, err := s.providerStore.Upsert(settings.ProviderInstanceInput{
		ID:           req.ID,
		KindID:       kindID,
		Label:        req.Label,
		Icon:         req.Icon,
		Enabled:      req.Enabled,
		DefaultModel: req.DefaultModel,
		Models:       req.Models,
		Config:       cfg,
		Secrets:      req.Secrets,
	})
	if err != nil {
		return settings.ProviderInstance{}, httpErr{http.StatusBadRequest, err.Error()}
	}
	return inst, nil
}

// handleUpsertProvider creates or updates a provider instance (providers.json,
// _Docs/71 §2.4), validates it against its kind's declared field schema, then
// re-applies settings so the registry picks up the write immediately — the
// fix for the silent create/registry disconnect this task was scoped around.
func (s *Server) handleUpsertProvider(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[upsertProviderInstanceReq](w, r)
	if !ok {
		return
	}
	_, exists := s.providerStore.Get(req.ID)
	creating := req.ID == "" || !exists
	inst, err := s.upsertProviderInstance(req, upsertProviderOpts{})
	if err != nil {
		if he, ok := err.(httpErr); ok {
			writeError(w, he.code, he.msg)
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if creating && (inst.KindID == "claude-cli" || inst.KindID == "codex-cli") && strings.TrimSpace(inst.Config[providers.FieldKeyConfigDir]) == "" {
		home := filepath.Join(filepath.Dir(s.providerStore.Path()), "provider-homes", inst.ID)
		if err := os.MkdirAll(home, 0o700); err != nil {
			_ = s.providerStore.Delete(inst.ID)
			writeError(w, http.StatusInternalServerError, "create provider home: "+err.Error())
			return
		}
		req.ID = inst.ID
		if req.Config == nil {
			req.Config = map[string]string{}
		}
		req.Config[providers.FieldKeyConfigDir] = home
		inst, err = s.upsertProviderInstance(req, upsertProviderOpts{})
		if err != nil {
			_ = s.providerStore.Delete(req.ID)
			writeError(w, http.StatusInternalServerError, "persist provider home: "+err.Error())
			return
		}
	}
	s.applySettings()
	s.logger.Info("provider instance upserted", "id", inst.ID, "kind", inst.KindID)
	writeJSON(w, http.StatusOK, inst.ToDTO())
}

// deleteProviderResp reports the instance ids that referenced the now-deleted
// provider so the caller is never silently left with orphaned agents
// (_Docs/71 Faz 3 item 5 — no silent swallow).
type deleteProviderResp struct {
	Deleted        bool     `json:"deleted"`
	AffectedAgents []string `json:"affectedAgents"`
}

// handleDeleteProvider removes a provider instance by id. It always deletes
// (the plan is silent on blocking the delete outright), but scans every
// workspace's agent roster first and reports every agent still pointing at
// this instance in the response — never swallowed.
func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.providerStore.Get(id); !ok {
		writeError(w, http.StatusNotFound, "provider instance not found")
		return
	}

	affected := s.agentsUsingProviderInstance(r.Context(), id)

	if err := s.providerStore.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.applySettings()
	s.logger.Info("provider instance deleted", "id", id, "affectedAgents", len(affected))
	writeJSON(w, http.StatusOK, deleteProviderResp{Deleted: true, AffectedAgents: affected})
}

// agentsUsingProviderInstance scans every open workspace's roster (including
// deleted agents, so a re-enabled agent isn't missed) for agents bound to the
// given provider instance id, resolving each agent's backfilled
// ProviderInstanceID exactly as the runtime does. Returns agent ids, not
// names, since agents span workspaces and only the id is unambiguous here.
func (s *Server) agentsUsingProviderInstance(ctx context.Context, id string) []string {
	var out []string
	for _, meta := range s.workspaces.List() {
		wsp, err := s.workspaces.Get(meta.ID)
		if err != nil {
			continue
		}
		agents, err := wsp.DB.ListAgentsWithDeleted(ctx)
		if err != nil {
			continue
		}
		for _, a := range agents {
			if a.ProviderInstanceID == id || (a.ProviderInstanceID == "" && a.Provider == id) {
				out = append(out, a.ID)
			}
		}
	}
	return out
}

// providerKindByID looks up a registered provider kind by its Manifest().Kind,
// walking providers.Kinds() rather than importing an internal registry map —
// keeps this file's only coupling to internal/providers the same public
// surface the catalog/schema handlers already use.
func providerKindByID(id string) (providers.ProviderKind, bool) {
	for _, k := range providers.Kinds() {
		if k.Manifest().Kind == id {
			return k, true
		}
	}
	return providers.ProviderKind(nil), false
}

// handlePrices returns the ballpark list-price table (provider id → model →
// {inputPerMTok, outputPerMTok, ...}) so the UI can show approximate $/1M-token
// costs (e.g. next to each model in the market provider preview). These are
// estimates, not billing-grade — see providers.priceTable.
func (s *Server) handlePrices(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, providers.AllPrices())
}
