package api

import (
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// registerEntityArchiveRoutes wires the archive/unarchive pair (see
// registerArchiveRoutes) for every entity that mirrors the kanban card's
// archive: skills, artifacts, automations and agents.
func (s *Server) registerEntityArchiveRoutes(mux *http.ServeMux) {
	s.registerArchiveRoutes(mux, archiveRoute{
		base: "/api/agents/{id}", param: "id", kind: "agent", event: "agent", view: "agents",
		set: func(r *http.Request, id string, archived bool) error {
			_, err := ws(r).DB.SetAgentArchived(r.Context(), id, archived)
			return err
		},
	})
	s.registerArchiveRoutes(mux, archiveRoute{
		base: "/api/artifacts/{id}", param: "id", kind: "artifact", event: "artifact", view: "artifacts",
		set: func(r *http.Request, id string, archived bool) error {
			_, err := ws(r).DB.SetArtifactArchived(r.Context(), id, archived)
			return err
		},
	})
	s.registerArchiveRoutes(mux, archiveRoute{
		base: "/api/automations/{id}", param: "id", kind: "automation", event: "automation", view: "schedules",
		set: func(r *http.Request, id string, archived bool) error {
			return ws(r).DB.SetAutomationArchived(r.Context(), id, archived)
		},
	})
	s.registerArchiveRoutes(mux, archiveRoute{
		base: "/api/skills/{slug}", param: "slug", kind: "skill", event: "skills", view: "skills",
		set: func(r *http.Request, slug string, archived bool) error {
			store := ws(r).Runtime.Skills()
			if _, ok := store.Get(slug); !ok {
				return db.ErrNotFound
			}
			_, err := store.SetArchived(slug, archived)
			return err
		},
	})
}
