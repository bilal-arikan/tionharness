package api

import (
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Curator + pin endpoints (Rota F3).
//
//	GET  /api/curator/report            → last pass (404 when none ran yet)
//	POST /api/curator/run?apply=false   → run now (apply defaults to true)
//	POST /api/automations/{id}/pin      {pinned}
//	POST /api/schedules/{id}/pin        {pinned}
//	POST /api/hooks/{id}/pin            {pinned}

func (s *Server) handleCuratorReport(w http.ResponseWriter, r *http.Request) {
	rep, ok, err := ws(r).DB.GetCuratorReport(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "curator has not run yet")
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleCuratorRun(w http.ResponseWriter, r *http.Request) {
	apply := r.URL.Query().Get("apply") != "false"
	rep, err := ws(r).Runtime.RunCurator(r.Context(), "manual", apply)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

type pinReq struct {
	Pinned bool `json:"pinned"`
}

func (s *Server) handlePinAutomation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[pinReq](w, r)
	if !ok {
		return
	}
	if err := ws(r).DB.SetAutomationPinned(r.Context(), id, req.Pinned); writeDBError(w, err, "automation not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "pinned": req.Pinned})
}

func (s *Server) handlePinSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[pinReq](w, r)
	if !ok {
		return
	}
	if err := ws(r).DB.SetSchedulePinned(r.Context(), id, req.Pinned); writeDBError(w, err, "schedule not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "pinned": req.Pinned})
}

func (s *Server) handlePinHook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[pinReq](w, r)
	if !ok {
		return
	}
	if err := ws(r).DB.SetHookPinned(r.Context(), id, req.Pinned); writeDBError(w, err, "hook not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "pinned": req.Pinned})
}

// handleRecipeStats serves the per-recipe-version rollup of the trajectory
// index (GET /api/trajectories/recipes?slug=…).
func (s *Server) handleRecipeStats(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	rows := wsp.DB.ListTrajectories(r.Context(), db.TrajectoryFilter{})
	stats := wsp.Runtime.RecipeStats(rows)
	if slug := r.URL.Query().Get("slug"); slug != "" {
		filtered := stats[:0]
		for _, st := range stats {
			if st.Slug == slug {
				filtered = append(filtered, st)
			}
		}
		stats = filtered
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleSummarizeTrajectory recomputes a trajectory's summary on demand
// (POST /api/trajectories/{id}/summarize) and returns the graph.
func (s *Server) handleSummarizeTrajectory(w http.ResponseWriter, r *http.Request) {
	t, err := ws(r).Runtime.SummarizeTrajectory(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "trajectory not found") {
		return
	}
	writeJSON(w, http.StatusOK, t)
}
