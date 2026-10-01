package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionharness/internal/decider"
)

// Decision-model settings (internal/decider): the master switch and default
// model, every authority's settings, the per-authority and per-model ledger
// numbers that decide when an authority can move from shadow to on, and the
// decision models themselves (decider_models.go).

const (
	deciderStatsDays   = 7
	deciderRecentLimit = 50
	// deciderTestDeadline leaves room for a local model's first call, which
	// may load the weights before answering.
	deciderTestDeadline = 60 * time.Second
)

func (s *Server) registerDeciderRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/decider", s.handleGetDecider)
	mux.HandleFunc("PUT /api/decider", s.handlePutDecider)
	mux.HandleFunc("POST /api/decider/test", s.handleTestDecider)
	mux.HandleFunc("GET /api/decider/stats", s.handleDeciderStats)
	mux.HandleFunc("GET /api/decider/debug", s.handleDeciderDebug)
	s.registerDeciderModelRoutes(mux)
}

// deciderModelView is one decision model as the settings page shows it: the
// masked settings, its health, its numbers, and what relies on it.
type deciderModelView struct {
	decider.ModelDTO
	Status decider.ModelStatus `json:"status"`
	Stats  *decider.ModelStats `json:"stats,omitempty"`
	// UsedBy lists "default" and the authorities naming the model.
	UsedBy []string `json:"usedBy"`
}

// deciderView is everything the settings page renders.
type deciderView struct {
	Config      decider.Config      `json:"config"`
	Status      decider.Status      `json:"status"`
	Backends    []decider.Manifest  `json:"backends"`
	Groups      []string            `json:"groups"`
	Authorities []decider.Authority `json:"authorities"`
	Models      []deciderModelView  `json:"models"`
	// ProviderCandidates lists, per backend, the provider accounts whose
	// credentials a model of that backend can borrow.
	ProviderCandidates map[string][]decider.InstanceInfo `json:"providerCandidates"`
	Stats              []decider.AuthorityStats          `json:"stats"`
	Recent             []decider.Record                  `json:"recent"`
	StatsDays          int                               `json:"statsDays"`
}

func (s *Server) deciderHubOrError(w http.ResponseWriter) *decider.Hub {
	hub := s.tun.Decider()
	if hub == nil {
		writeError(w, http.StatusServiceUnavailable, "decision-model layer is not available")
	}
	return hub
}

func (s *Server) deciderView(hub *decider.Hub, days int) deciderView {
	window := time.Duration(days) * 24 * time.Hour
	modelStats := map[string]decider.ModelStats{}
	for _, st := range hub.ModelStats(window) {
		modelStats[st.Instance] = st
	}
	models := hub.Models()
	views := make([]deciderModelView, 0, len(models))
	for _, m := range models {
		v := deciderModelView{ModelDTO: m.ToDTO(), Status: hub.ModelStatus(m.ID), UsedBy: nonNil(hub.ModelUsers(m.ID))}
		if st, ok := modelStats[m.ID]; ok {
			v.Stats = &st
		}
		views = append(views, v)
	}
	backends := decider.Manifests()
	candidates := make(map[string][]decider.InstanceInfo, len(backends))
	for _, b := range backends {
		candidates[b.ID] = nonNil(hub.ProviderCandidates(b.ID))
	}
	return deciderView{
		Config:             hub.Config(),
		Status:             hub.Status(),
		Backends:           backends,
		Groups:             decider.Groups(),
		Authorities:        decider.Authorities(),
		Models:             views,
		ProviderCandidates: candidates,
		Stats:              nonNil(hub.Stats(window)),
		Recent:             nonNil(hub.Recent(deciderRecentLimit)),
		StatsDays:          days,
	}
}

func (s *Server) handleGetDecider(w http.ResponseWriter, r *http.Request) {
	hub := s.deciderHubOrError(w)
	if hub == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.deciderView(hub, deciderStatsDays))
}

func (s *Server) handlePutDecider(w http.ResponseWriter, r *http.Request) {
	hub := s.deciderHubOrError(w)
	if hub == nil {
		return
	}
	cfg, ok := bindJSONStrict[decider.Config](w, r)
	if !ok {
		return
	}
	saved, err := hub.Update(cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("decider settings updated", "enabled", saved.Enabled, "defaultModel", saved.DefaultModel)
	writeJSON(w, http.StatusOK, s.deciderView(hub, deciderStatsDays))
}

// deciderTestResult is the outcome of a test button. A failed call is a 200
// with ok=false: the request itself worked, the configuration is what the page
// is reporting on.
type deciderTestResult struct {
	OK       bool              `json:"ok"`
	Error    string            `json:"error,omitempty"`
	Response *decider.Response `json:"response,omitempty"`
}

// handleTestDecider tests the default model.
func (s *Server) handleTestDecider(w http.ResponseWriter, r *http.Request) {
	s.runDeciderTest(w, r, "")
}

func (s *Server) runDeciderTest(w http.ResponseWriter, r *http.Request, id string) {
	hub := s.deciderHubOrError(w)
	if hub == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), deciderTestDeadline)
	defer cancel()
	resp, err := hub.Test(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusOK, deciderTestResult{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, deciderTestResult{OK: true, Response: resp})
}

func (s *Server) handleDeciderStats(w http.ResponseWriter, r *http.Request) {
	hub := s.deciderHubOrError(w)
	if hub == nil {
		return
	}
	days := deciderStatsDays
	if v, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && v > 0 && v <= 90 {
		days = v
	}
	view := s.deciderView(hub, days)
	writeJSON(w, http.StatusOK, map[string]any{"stats": view.Stats, "recent": view.Recent, "statsDays": days})
}

// nonNil turns a nil slice into an empty one so the JSON is [] rather than null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
