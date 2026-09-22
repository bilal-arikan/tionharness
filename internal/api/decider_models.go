package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/decider"
)

// Decision models ("karar modelleri"): created, edited and deleted like
// provider instances. Every mutation answers with the whole settings view, so
// the page never has to stitch partial updates together.

func (s *Server) registerDeciderModelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/decider/models", s.handleCreateDeciderModel)
	mux.HandleFunc("PUT /api/decider/models/{id}", s.handleUpdateDeciderModel)
	mux.HandleFunc("DELETE /api/decider/models/{id}", s.handleDeleteDeciderModel)
	mux.HandleFunc("POST /api/decider/models/{id}/test", s.handleTestDeciderModel)
}

// deciderModelSaved answers a create or update.
type deciderModelSaved struct {
	Model decider.ModelDTO `json:"model"`
	View  deciderView      `json:"view"`
}

// deciderModelDeleted answers a delete. UsedBy lists what relied on the model
// ("default", authority ids): those now use the default model — reported, never
// swallowed, like a deleted provider's affected agents.
type deciderModelDeleted struct {
	Deleted bool        `json:"deleted"`
	UsedBy  []string    `json:"usedBy"`
	View    deciderView `json:"view"`
}

func (s *Server) handleCreateDeciderModel(w http.ResponseWriter, r *http.Request) {
	hub := s.deciderHubOrError(w)
	if hub == nil {
		return
	}
	in, ok := bindJSONStrict[decider.ModelInput](w, r)
	if !ok {
		return
	}
	if id := strings.TrimSpace(in.ID); id != "" {
		if _, exists := hub.Model(id); exists {
			writeError(w, http.StatusConflict, "decision model "+id+" already exists")
			return
		}
	}
	s.saveDeciderModel(w, hub, in)
}

func (s *Server) handleUpdateDeciderModel(w http.ResponseWriter, r *http.Request) {
	hub := s.deciderHubOrError(w)
	if hub == nil {
		return
	}
	id := r.PathValue("id")
	if _, exists := hub.Model(id); !exists {
		writeError(w, http.StatusNotFound, "decision model not found")
		return
	}
	in, ok := bindJSONStrict[decider.ModelInput](w, r)
	if !ok {
		return
	}
	in.ID = id
	s.saveDeciderModel(w, hub, in)
}

func (s *Server) saveDeciderModel(w http.ResponseWriter, hub *decider.Hub, in decider.ModelInput) {
	m, err := hub.UpsertModel(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("decision model saved", "id", m.ID, "backend", m.Backend, "model", m.Model, "credentials", m.Credentials, "enabled", m.Enabled)
	writeJSON(w, http.StatusOK, deciderModelSaved{Model: m.ToDTO(), View: s.deciderView(hub, deciderStatsDays)})
}

func (s *Server) handleDeleteDeciderModel(w http.ResponseWriter, r *http.Request) {
	hub := s.deciderHubOrError(w)
	if hub == nil {
		return
	}
	id := r.PathValue("id")
	if _, exists := hub.Model(id); !exists {
		writeError(w, http.StatusNotFound, "decision model not found")
		return
	}
	users, err := hub.DeleteModel(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("decision model deleted", "id", id, "usedBy", users)
	writeJSON(w, http.StatusOK, deciderModelDeleted{Deleted: true, UsedBy: nonNil(users), View: s.deciderView(hub, deciderStatsDays)})
}

func (s *Server) handleTestDeciderModel(w http.ResponseWriter, r *http.Request) {
	hub := s.deciderHubOrError(w)
	if hub == nil {
		return
	}
	id := r.PathValue("id")
	if _, exists := hub.Model(id); !exists {
		writeError(w, http.StatusNotFound, "decision model not found")
		return
	}
	s.runDeciderTest(w, r, id)
}
