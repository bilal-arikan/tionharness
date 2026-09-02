package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Trajectory ("Rota") reads (_Docs/77 R4, Rota F0). Both are read-only: the
// trajectory is a projection that runtime observers write; the UI and agents
// only read it. The list serves the index rows (no sidecar is opened), the
// single read returns the full graph.
//
//	GET /api/trajectories?root=SES1&template=plan-dev-test@1&status=running&terminal=true|false&limit=N
//	GET /api/trajectories/{id}

const trajectoryListDefaultLimit = 200

func (s *Server) handleListTrajectories(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	q := r.URL.Query()
	f := db.TrajectoryFilter{
		RootSessionID: q.Get("root"),
		TemplateRef:   q.Get("template"),
		Status:        q.Get("status"),
	}
	if t := q.Get("terminal"); t != "" {
		v := t == "true" || t == "1"
		f.Terminal = &v
	}
	rows := wsp.DB.ListTrajectories(r.Context(), f)
	limit := trajectoryListDefaultLimit
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		limit = n
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	if rows == nil {
		rows = []db.TrajectoryIndexEntry{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleGetTrajectory(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	t, err := wsp.DB.GetTrajectory(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "trajectory not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}
