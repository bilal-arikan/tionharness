package api

import (
	"errors"
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/goals"
)

// Evolution goals (_Docs/83 §4.1).
//
//	GET    /api/goals                → every goal (?status= filters)
//	GET    /api/goals/catalog        → metric catalog + scope candidates
//	POST   /api/goals                → user creates a goal directly (validated)
//	POST   /api/goals/intake         → {text, goalId?}: the goal-writer agent drafts / rewrites a goal
//	GET    /api/goals/{id}
//	PUT    /api/goals/{id}           → user edit (validated; appends a revision)
//	POST   /api/goals/{id}/status    → {status}: draft | active | paused | archived
//	DELETE /api/goals/{id}

func (s *Server) handleListGoals(w http.ResponseWriter, r *http.Request) {
	var (
		list []db.Goal
		err  error
	)
	if st := r.URL.Query().Get("status"); st != "" {
		list, err = ws(r).DB.ListGoalsByStatus(r.Context(), st)
	} else {
		list, err = ws(r).DB.ListGoals(r.Context())
	}
	if writeDBError(w, err, "") {
		return
	}
	if list == nil {
		list = []db.Goal{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGoalCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"metrics":    goals.Catalog(),
		"candidates": ws(r).Runtime.GoalScopeCandidates(r.Context()),
	})
}

// handleCreateGoal stores a goal the user built in the editor. Provenance,
// id and history are the store's; a direct goal carries no RawText.
func (s *Server) handleCreateGoal(w http.ResponseWriter, r *http.Request) {
	var g db.Goal
	if err := decodeJSON(r, &g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	g.RawText, g.CreatedBy, g.History = "", "", nil
	goals.Normalize(&g)
	if err := goals.Validate(g); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	stored, err := ws(r).DB.CreateGoal(r.Context(), g, db.GoalByUser, "created in the editor")
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusCreated, stored)
}

func (s *Server) handleGoalIntake(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text   string `json:"text"`
		GoalID string `json:"goalId"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	res, err := ws(r).Runtime.WriteGoal(r.Context(), body.Text, body.GoalID)
	switch {
	case err == nil:
	case errors.Is(err, goals.ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case errors.Is(err, db.ErrNotFound):
		writeError(w, http.StatusNotFound, "goal not found")
		return
	default:
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	status := http.StatusOK
	if res.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, res)
}

func (s *Server) handleGetGoal(w http.ResponseWriter, r *http.Request) {
	g, err := ws(r).DB.GetGoal(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "goal not found") {
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleUpdateGoal(w http.ResponseWriter, r *http.Request) {
	var g db.Goal
	if err := decodeJSON(r, &g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	g.ID = r.PathValue("id")
	goals.Normalize(&g)
	if err := goals.Validate(g); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	stored, err := ws(r).DB.UpdateGoal(r.Context(), g, db.GoalByUser, "")
	if writeDBError(w, err, "goal not found") {
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (s *Server) handleSetGoalStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	switch body.Status {
	case db.GoalStatusDraft, db.GoalStatusActive, db.GoalStatusPaused, db.GoalStatusArchived:
	default:
		writeError(w, http.StatusBadRequest, "unknown status")
		return
	}
	g, err := ws(r).DB.SetGoalStatus(r.Context(), r.PathValue("id"), body.Status, db.GoalByUser)
	if writeDBError(w, err, "goal not found") {
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleDeleteGoal(w http.ResponseWriter, r *http.Request) {
	if writeDBError(w, ws(r).DB.DeleteGoal(r.Context(), r.PathValue("id")), "goal not found") {
		return
	}
	_ = ws(r).DB.DeleteEvolutionGoalState(r.Context(), r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}
