package api

import (
	"net/http"

	"github.com/bilal-arikan/swarmgo/internal/providers"
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
	entries = append(entries, s.providers.CustomCatalog()...)
	out := make([]catalogEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, catalogEntryDTO{CatalogEntry: e, Available: s.providers.Available(e.ID)})
	}
	writeJSON(w, http.StatusOK, out)
}
