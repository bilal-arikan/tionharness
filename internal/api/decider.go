package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionharness/internal/decider"
)

// Decision-model settings (internal/decider): configuration, health, the
// per-site agreement/latency/cost stats that decide when a site can move from
// shadow to on, and a live test call.

const (
	deciderStatsDays    = 7
	deciderRecentLimit  = 50
	deciderTestDeadline = 15 * time.Second
)

func (s *Server) registerDeciderRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/decider", s.handleGetDecider)
	mux.HandleFunc("PUT /api/decider", s.handlePutDecider)
	mux.HandleFunc("POST /api/decider/test", s.handleTestDecider)
	mux.HandleFunc("GET /api/decider/stats", s.handleDeciderStats)
}

// deciderView is everything the settings page renders.
type deciderView struct {
	Config     decider.Config         `json:"config"`
	Status     decider.Status         `json:"status"`
	Backends   []decider.Manifest     `json:"backends"`
	Sites      []decider.Site         `json:"sites"`
	Candidates []decider.InstanceInfo `json:"candidates"`
	Stats      []decider.SiteStats    `json:"stats"`
	Recent     []decider.Record       `json:"recent"`
	StatsDays  int                    `json:"statsDays"`
}

func (s *Server) deciderHubOrError(w http.ResponseWriter) *decider.Hub {
	hub := s.tun.Decider()
	if hub == nil {
		writeError(w, http.StatusServiceUnavailable, "decision-model layer is not available")
	}
	return hub
}

func (s *Server) deciderView(hub *decider.Hub, days int) deciderView {
	return deciderView{
		Config:     hub.Config(),
		Status:     hub.Status(),
		Backends:   decider.Manifests(),
		Sites:      decider.Sites(),
		Candidates: nonNil(hub.Candidates()),
		Stats:      nonNil(hub.Stats(time.Duration(days) * 24 * time.Hour)),
		Recent:     nonNil(hub.Recent(deciderRecentLimit)),
		StatsDays:  days,
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
	if _, err := hub.Update(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("decider settings updated", "enabled", cfg.Enabled, "backend", cfg.Backend, "model", cfg.Model, "instance", cfg.ProviderInstanceID)
	writeJSON(w, http.StatusOK, s.deciderView(hub, deciderStatsDays))
}

// deciderTestResult is the outcome of the settings page's test button. A failed
// call is a 200 with ok=false: the request itself worked, the configuration is
// what the page is reporting on.
type deciderTestResult struct {
	OK       bool              `json:"ok"`
	Error    string            `json:"error,omitempty"`
	Response *decider.Response `json:"response,omitempty"`
}

func (s *Server) handleTestDecider(w http.ResponseWriter, r *http.Request) {
	hub := s.deciderHubOrError(w)
	if hub == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), deciderTestDeadline)
	defer cancel()
	resp, err := hub.Test(ctx)
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
