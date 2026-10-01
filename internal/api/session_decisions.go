package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

var errRequiredDecisionContext = errors.New("required context cannot be unpinned")

func (s *Server) handleSessionDecisions(w http.ResponseWriter, r *http.Request) {
	state, err := ws(r).DB.ReadSessionDecisions(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "") {
		return
	}
	if state.Route == nil {
		if session, e := ws(r).DB.GetSession(r.Context(), r.PathValue("id")); e == nil {
			if a, e := ws(r).DB.GetAgent(r.Context(), session.OwnerAgentID()); e == nil {
				model := session.Model
				if model == "" {
					model = a.Model
				}
				state.Route = &db.DecisionRoute{Provider: a.ProviderRef(), Model: model}
			}
		}
	}
	// Raw saved text is private context; the inspector needs labels and source
	// provenance only. The canonical transcript already exposes full messages.
	for i := range state.Memories {
		state.Memories[i].Text = ""
	}
	state.SkillBodies = nil
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleSessionDecisionPin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Key    string `json:"key"`
		Pinned bool   `json:"pinned"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil || strings.TrimSpace(input.Key) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A valid context key is required."})
		return
	}
	store := ws(r).DB
	id := r.PathValue("id")
	var baseline *db.DecisionRoute
	if input.Key == "model" {
		session, err := store.GetSession(r.Context(), id)
		if writeDBError(w, err, "") {
			return
		}
		a, err := store.GetAgent(r.Context(), session.OwnerAgentID())
		if writeDBError(w, err, "") {
			return
		}
		model := session.Model
		if model == "" {
			model = a.Model
		}
		baseline = &db.DecisionRoute{Provider: a.ProviderRef(), Model: model}
	}
	err := store.UpdateSessionDecisions(r.Context(), id, func(state *db.SessionDecisions) error {
		if input.Key == "model" {
			if state.Route == nil {
				state.Route = baseline
			}
			state.Route.Pinned = input.Pinned
			return nil
		}
		for i, m := range state.Memories {
			if m.Key == input.Key {
				if m.Mandatory && !input.Pinned {
					return errRequiredDecisionContext
				}
				state.Memories[i].Pinned = input.Pinned
				return nil
			}
		}
		return db.ErrNotFound
	})
	if errors.Is(err, errRequiredDecisionContext) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if writeDBError(w, err, "") {
		return
	}
	s.handleSessionDecisions(w, r)
}

func (s *Server) handleSessionDecisionFeedback(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DecisionID string `json:"decisionId"`
		Rating     string `json:"rating"`
		Note       string `json:"note"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil || input.DecisionID == "" || (input.Rating != "helpful" && input.Rating != "correction") || len(input.Note) > 1000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A decision and a helpful or correction rating are required."})
		return
	}
	err := ws(r).DB.UpdateSessionDecisions(r.Context(), r.PathValue("id"), func(state *db.SessionDecisions) error {
		found := false
		for _, e := range state.Entries {
			if e.ID == input.DecisionID {
				found = true
				break
			}
		}
		if !found {
			return db.ErrNotFound
		}
		feedback := db.DecisionFeedback{DecisionID: input.DecisionID, Rating: input.Rating, Note: strings.TrimSpace(input.Note), At: time.Now().UnixMilli()}
		for i, f := range state.Feedback {
			if f.DecisionID == input.DecisionID {
				state.Feedback[i] = feedback
				return nil
			}
		}
		state.Feedback = append(state.Feedback, feedback)
		return nil
	})
	if writeDBError(w, err, "") {
		return
	}
	s.handleSessionDecisions(w, r)
}
