package api

import (
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/settings"
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

// upsertProviderReq is the create/update payload. Key is write-only: omitted
// (nil) keeps the stored key, "" clears it, a value replaces it.
type upsertProviderReq struct {
	ID           string  `json:"id"`
	Label        string  `json:"label"`
	Kind         string  `json:"kind"`
	BaseURL      string  `json:"baseUrl"`
	DefaultModel string  `json:"defaultModel"`
	Models       string  `json:"models"`
	Key          *string `json:"key"`
	Reasoning    bool    `json:"reasoning"`
	PromptCache  string  `json:"promptCache"`
}

// handleListProviders returns the masked custom-provider list.
func (s *Server) handleListProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.settings.DTO().CustomProviders)
}

// handlePrices returns the ballpark list-price table (provider id → model →
// {inputPerMTok, outputPerMTok, ...}) so the UI can show approximate $/1M-token
// costs (e.g. next to each model in the market provider preview). These are
// estimates, not billing-grade — see providers.priceTable.
func (s *Server) handlePrices(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, providers.AllPrices())
}

// handleUpsertProvider creates or updates a custom provider, then re-applies
// settings so the registry picks it up immediately.
func (s *Server) handleUpsertProvider(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[upsertProviderReq](w, r)
	if !ok {
		return
	}
	_, err := s.settings.UpsertCustomProvider(settings.CustomProvider{
		ID:           req.ID,
		Label:        req.Label,
		Kind:         req.Kind,
		BaseURL:      req.BaseURL,
		DefaultModel: req.DefaultModel,
		Models:       req.Models,
		Reasoning:    req.Reasoning,
		PromptCache:  req.PromptCache,
	}, req.Key)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.applySettings()
	s.logger.Info("custom provider upserted", "id", req.ID, "kind", req.Kind)
	writeJSON(w, http.StatusOK, s.settings.DTO().CustomProviders)
}

// handleDeleteProvider removes a custom provider by id.
func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.settings.DeleteCustomProvider(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.applySettings()
	s.logger.Info("custom provider deleted", "id", id)
	writeJSON(w, http.StatusOK, s.settings.DTO().CustomProviders)
}
