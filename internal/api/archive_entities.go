package api

import (
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// registerEntityArchiveRoutes wires the archive/unarchive pair (see
// registerArchiveRoutes) for every entity that mirrors the kanban card's
// archive: skills, artifacts, automations, goals and agents.
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
		base: "/api/goals/{id}", param: "id", kind: "goal", view: "goals",
		set: setGoalArchived,
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

// setGoalArchived maps the archive pair onto a goal's status, which already
// carries the archived state (GoalStatusArchived). Archiving moves the goal to
// "archived"; unarchiving an archived goal returns it to "draft" — the same
// transition the Goals screen offers — so a restored goal never resumes
// autonomous evolution without the user re-activating it. Unarchiving a goal
// that is not archived is a no-op.
func setGoalArchived(r *http.Request, id string, archived bool) error {
	database := ws(r).DB
	g, err := database.GetGoal(r.Context(), id)
	if err != nil {
		return err
	}
	switch {
	case archived:
		_, err = database.SetGoalStatus(r.Context(), id, db.GoalStatusArchived, db.GoalByUser)
	case g.Status == db.GoalStatusArchived:
		_, err = database.SetGoalStatus(r.Context(), id, db.GoalStatusDraft, db.GoalByUser)
	}
	return err
}
