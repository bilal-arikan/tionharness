package api

import (
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Recipe optimizer endpoints (Rota F4).
//
//	GET  /api/recipes/{slug}/optimizer → last pass bookkeeping for the slug
//	POST /api/recipes/{slug}/optimize  → run the optimizer now (manual trigger)

func (s *Server) handleRecipeOptimizerState(w http.ResponseWriter, r *http.Request) {
	st, err := ws(r).DB.GetOptimizerState(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slug := r.PathValue("slug")
	row, ok := st.Slugs[slug]
	if !ok {
		row = db.OptimizerSlugState{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"slug": slug, "ran": ok, "state": row})
}

func (s *Server) handleOptimizeRecipe(w http.ResponseWriter, r *http.Request) {
	res, err := ws(r).Runtime.RunRecipeOptimizer(r.Context(), r.PathValue("slug"), "manual")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}
