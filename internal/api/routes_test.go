package api

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRouteTableServesFrontendAPIPaths pins that every /api path the frontend
// calls resolves to an API pattern rather than falling through to the SPA
// catch-all. The failure mode is silent: the catch-all answers 200 with
// index.html, the client's JSON parse fails, and the screen reports a generic
// error — which is how the Explorer and Panel screens went dark when a route
// group was rewritten and the view + dashboard registrations were dropped
// with it (2026-10-04).
//
// The list is the frontend's GET surface per feature (frontend/src/api/*.ts);
// add a line here when a screen gains a new endpoint.
func TestRouteTableServesFrontendAPIPaths(t *testing.T) {
	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	mux := s.routeTable()

	paths := []string{
		// Explorer map + the projection side panel (features/explorer, features/view).
		"/api/views/graph",
		"/api/views/workspace/workspace",
		"/api/views/session/SES1/children",
		"/api/views/session/SES1/neighborhood",
		// Panel screen (features/dashboard).
		"/api/dashboard",
		"/api/dashboard/commit-activity",
		// Notes + awareness (features/notes).
		"/api/notes",
		"/api/notes/stats",
		"/api/notes/search",
		"/api/notes/NOTE1",
		"/api/notes/NOTE1/expand",
		"/api/awareness/digests",
		"/api/awareness/settings",
		"/api/sessions/SES1/digest",
		"/api/sessions/SES1/awareness",
		// Core screens.
		"/api/agents",
		"/api/sessions",
		"/api/tasks",
		"/api/schedules",
		"/api/flows",
		"/api/artifacts",
		"/api/skills",
		"/api/usage",
		"/api/workspaces",
		"/api/workspace-settings",
		"/api/settings",
		"/api/decider",
		"/api/decider/stats",
		"/health",
	}
	for _, p := range paths {
		req := httptest.NewRequest("GET", p, nil)
		_, pattern := mux.Handler(req)
		if !strings.HasPrefix(pattern, "GET /api/") && pattern != "GET /health" {
			t.Errorf("%s resolves to %q, want an API pattern (not the SPA catch-all)", p, pattern)
		}
	}
}
