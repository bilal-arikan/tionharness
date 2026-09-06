package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/goals"
	"github.com/bilal-arikan/tionharness/internal/insight"
)

// Evolution fitness + configuration history (_Docs/83 §4.2, E1).
//
//	GET /api/goals/{id}/fitness?since=<unix>   → the goal's metrics, current + per snapshot
//	POST /api/goals/{id}/evolve                → run the evolver now (manual trigger)
//	GET /api/goals/{id}/evolution              → last pass bookkeeping + this goal's open proposals
//	GET /api/evolution/snapshots               → configuration versions, oldest first, with diffs
//	GET /api/evolution/snapshots/{hash}        → one snapshot's content

const fitnessDefaultWindow = 30 * 24 * time.Hour

func (s *Server) handleGoalFitness(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	g, err := wsp.DB.GetGoal(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "goal not found") {
		return
	}
	now := time.Now().Unix()
	since := now - int64(fitnessDefaultWindow/time.Second)
	if raw := r.URL.Query().Get("since"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v >= 0 {
			since = v
		}
	}
	if wsp.Runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime not available")
		return
	}
	in := wsp.Runtime.FitnessInputs(r.Context(), now, since)
	writeJSON(w, http.StatusOK, goals.Evaluate(g, in))
}

// snapshotListRow is one configuration version for the history view.
type snapshotListRow struct {
	db.SnapshotIndexEntry
	Current  bool                   `json:"current"`
	Sessions int                    `json:"sessions"`
	Changes  []goals.SnapshotChange `json:"changes,omitempty"` // vs Prev
}

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	if wsp.Runtime != nil {
		_ = wsp.Runtime.CurrentSnapshotHash() // make sure the live config is indexed
	}
	rows, current, err := wsp.DB.ListSnapshots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	counts := wsp.DB.CountSessionsBySnapshot(r.Context())
	cache := map[string]*goals.ConfigSnapshot{}
	load := func(h string) *goals.ConfigSnapshot {
		if h == "" {
			return nil
		}
		if c, ok := cache[h]; ok {
			return c
		}
		var snap goals.ConfigSnapshot
		if err := wsp.DB.GetSnapshot(r.Context(), h, &snap); err != nil {
			cache[h] = nil
			return nil
		}
		cache[h] = &snap
		return &snap
	}
	out := make([]snapshotListRow, 0, len(rows))
	for _, e := range rows {
		row := snapshotListRow{SnapshotIndexEntry: e, Current: e.Hash == current, Sessions: counts[e.Hash]}
		if prev, cur := load(e.Prev), load(e.Hash); prev != nil && cur != nil {
			row.Changes = goals.Diff(*prev, *cur)
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": current, "unstamped": counts[""], "snapshots": out})
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	var snap goals.ConfigSnapshot
	if writeDBError(w, ws(r).DB.GetSnapshot(r.Context(), r.PathValue("hash"), &snap), "snapshot not found") {
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleEvolveGoal(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	if wsp.Runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime not available")
		return
	}
	res, err := wsp.Runtime.RunGoalEvolver(r.Context(), r.PathValue("id"), "manual")
	switch {
	case err == nil:
	case errors.Is(err, db.ErrNotFound):
		writeError(w, http.StatusNotFound, "goal not found")
		return
	default:
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGoalEvolution(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	id := r.PathValue("id")
	if _, err := wsp.DB.GetGoal(r.Context(), id); writeDBError(w, err, "goal not found") {
		return
	}
	st, err := wsp.DB.GetEvolutionState(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	state, ran := st.Goals[id]
	store, err := insight.OpenFindingStore(wsp.DB.Root())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	findings := make([]insight.Finding, 0)
	for _, f := range store.List("", insight.ChannelEvolution) {
		if f.Evolution != nil && f.Evolution.GoalID == id {
			findings = append(findings, f)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"goalId": id, "ran": ran, "state": state, "findings": findings,
		"minRuns": goals.EffectiveMinRuns(db.Goal{}), "maxProposals": goals.MaxProposalsPerPass,
	})
}
