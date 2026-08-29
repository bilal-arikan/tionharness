package api

import (
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// catalogEntryDTO is a catalog entry plus whether the provider is configured
// and usable right now.
type catalogEntryDTO struct {
	providers.CatalogEntry
	Available bool `json:"available"`
	// CliVersion and Subscription describe the local CLI install behind the
	// claude-cli / codex-cli providers; both stay empty for every other provider.
	// Those providers' models are bare aliases ("sonnet") or bare OpenAI slugs, so
	// the pickers show the CLI version — and the subscription it runs on — beside
	// the model to make clear what is actually going to answer.
	CliVersion   string `json:"cliVersion,omitempty"`
	Subscription string `json:"subscription,omitempty"`
}

// handleCatalog returns the provider/model catalog with live availability so the
// UI's provider/model pickers can show which providers are ready to use.
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	// Custom (user-added) providers override any built-in that shares their ID
	// (a user configuring their own "openrouter" replaces the built-in default
	// instead of producing a duplicate catalog entry — see MergeCatalog).
	instanceEntries := s.providers.InstanceCatalog()
	entries := providers.MergeCatalog(providers.Catalog(), instanceEntries)

	// A TemplateOnly kind (openai-compat, anthropic-compat) exists only to be
	// instantiated from — it ships with no default/migrated instance, so its
	// per-kind entry is a permanently unavailable, model-less placeholder
	// unless MergeCatalog already replaced it with a same-ID instance above.
	// Drop the ones nothing replaced instead of showing that noise in the
	// model picker (provider-kinds — not this endpoint — is where a user
	// picks a template to build a new instance from).
	hasInstance := make(map[string]bool, len(instanceEntries))
	for _, e := range instanceEntries {
		hasInstance[e.ID] = true
	}
	templateOnlyKind := make(map[string]bool)
	for _, k := range providers.Kinds() {
		if k.Manifest().TemplateOnly {
			templateOnlyKind[k.Manifest().Kind] = true
		}
	}
	filtered := entries[:0]
	for _, e := range entries {
		if templateOnlyKind[e.ID] && !hasInstance[e.ID] {
			continue
		}
		filtered = append(filtered, e)
	}
	entries = filtered

	out := make([]catalogEntryDTO, 0, len(entries))
	wsp := ws(r)
	for _, e := range entries {
		dto := catalogEntryDTO{CatalogEntry: e, Available: s.providers.Available(e.ID)}
		// Annotate each model with the concrete id this workspace last observed
		// behind it. Only aliases carry one (claude-cli's "opus"); native providers
		// echo the id they were given, which the store filters out as no news.
		if wsp != nil {
			models := make([]providers.ModelInfo, len(e.Models))
			copy(models, e.Models)
			for i := range models {
				resolved := wsp.DB.ResolvedModelFor(e.ID, models[i].ID)
				if resolved == "" {
					// This workspace has never completed a turn with the alias, but
					// another one on the same machine may have. Falling back to the
					// app-global store is what keeps a freshly created workspace from
					// showing "Varsayılan" for claude-cli's empty model id.
					resolved = wsp.DB.GlobalResolvedModelFor(e.ID, models[i].ID)
				}
				models[i].ResolvedModel = resolved
			}
			dto.Models = models
		}
		if e.ID == "claude-cli" && dto.Available {
			dto.CliVersion = claudeCLIVersion(r.Context(), s.providers.ClaudeCLIPath())
			home, ok := s.resolveAppCLIHome(w, "claude-cli")
			if !ok {
				return
			}
			dto.Subscription = claudeSubscriptionTier(
				home,
				s.settings.Get().ClaudeCliAuthKind,
			)
		}
		if e.ID == "codex-cli" && dto.Available {
			dto.CliVersion = codexCLIVersion(r.Context(), s.providers.CodexCLIPath())
			home, ok := s.resolveAppCLIHome(w, "codex-cli")
			if !ok {
				return
			}
			dto.Subscription = codexSubscriptionTier(home)
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, out)
}
