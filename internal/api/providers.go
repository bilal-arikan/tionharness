package api

import (
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/settings"
)

// customProviderSpecs projects persisted custom providers into registry specs,
// decrypting each key. Used by applySettings to push them into the registry.
func (s *Server) customProviderSpecs(cur settings.Settings) []providers.CustomSpec {
	out := make([]providers.CustomSpec, 0, len(cur.CustomProviders))
	for _, c := range cur.CustomProviders {
		out = append(out, providers.CustomSpec{
			ID:           c.ID,
			Label:        c.Label,
			Kind:         c.Kind,
			BaseURL:      c.BaseURL,
			DefaultModel: c.DefaultModel,
			Models:       c.Models,
			Key:          s.settings.CustomProviderKey(c.ID),
			Reasoning:    c.Reasoning,
			PromptCache:  c.PromptCache,
		})
	}
	return out
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
