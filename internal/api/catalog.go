package api

import (
	"net/http"

	"github.com/bilal/swarmgo/internal/providers"
)

// catalogEntryDTO is a catalog entry plus whether the provider is configured
// and usable right now.
type catalogEntryDTO struct {
	providers.CatalogEntry
	Available bool `json:"available"`
}

// handleCatalog returns the provider/model catalog with live availability so the
// UI's provider/model pickers can show which providers are ready to use.
func (s *Server) handleCatalog(w http.ResponseWriter, _ *http.Request) {
	entries := providers.Catalog()
	out := make([]catalogEntryDTO, 0, len(entries))
	for _, e := range entries {
		available := false
		switch e.ID {
		case "claude-cli":
			available = s.providers.ClaudeCLIAvailable()
		case "anthropic":
			available = s.providers.AnthropicConfigured()
		case "minimax":
			available = s.providers.MinimaxConfigured()
		}
		out = append(out, catalogEntryDTO{CatalogEntry: e, Available: available})
	}
	writeJSON(w, http.StatusOK, out)
}
