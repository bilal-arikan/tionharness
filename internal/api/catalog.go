package api

import (
	"net/http"
	"path/filepath"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// catalogEntryDTO is a catalog entry plus whether the provider is configured
// and usable right now.
type catalogEntryDTO struct {
	providers.CatalogEntry
	Available bool `json:"available"`
	// CliVersion and Subscription describe the local Claude Code install behind
	// the claude-cli provider; both stay empty for every other provider. That
	// provider's models are bare aliases ("sonnet"), so the pickers show the CLI
	// version — and the Max/Pro plan it runs on — beside the model to make clear
	// what is actually going to answer.
	CliVersion   string `json:"cliVersion,omitempty"`
	Subscription string `json:"subscription,omitempty"`
}

// handleCatalog returns the provider/model catalog with live availability so the
// UI's provider/model pickers can show which providers are ready to use.
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	// Custom (user-added) providers override any built-in that shares their ID
	// (a user configuring their own "openrouter" replaces the built-in default
	// instead of producing a duplicate catalog entry — see MergeCatalog).
	entries := providers.MergeCatalog(providers.Catalog(), s.providers.CustomCatalog())
	out := make([]catalogEntryDTO, 0, len(entries))
	for _, e := range entries {
		dto := catalogEntryDTO{CatalogEntry: e, Available: s.providers.Available(e.ID)}
		if e.ID == "claude-cli" && dto.Available {
			dto.CliVersion = claudeCLIVersion(r.Context(), s.providers.ClaudeCLIPath())
			// The login lives in THIS workspace's claude-home, so the tier is
			// per-workspace too (one workspace may be on Max, another on an API key).
			if wsp := ws(r); wsp != nil {
				dto.Subscription = claudeSubscriptionTier(
					filepath.Join(wsp.DataDir, "claude-home"),
					s.settings.Get().ClaudeCliAuthKind,
				)
			}
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, out)
}
