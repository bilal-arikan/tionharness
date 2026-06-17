package api

import (
	"net/http"

	"github.com/bilal/swarmgo/internal/skills"
)

// skillDetail is a skill plus its (lazily read) markdown body, returned by the
// detail endpoint so the Skills screen can render full instructions on demand.
type skillDetail struct {
	skills.Skill
	Body string `json:"body"`
}

// handleListSkills returns the resolved skill catalog (frontmatter only) for the
// workspace, newest tier winning on slug collisions.
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	store := ws(r).Runtime.Skills()
	list := store.List()
	if list == nil {
		list = []skills.Skill{}
	}
	writeJSON(w, http.StatusOK, list)
}

// handleGetSkill returns one skill plus its full markdown body. The body is read
// from disk on request — it is never carried in the catalog.
func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	store := ws(r).Runtime.Skills()
	sk, ok := store.Get(slug)
	if !ok {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	body, err := store.Body(slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, skillDetail{Skill: sk, Body: body})
}

// handleReloadSkills re-scans the skill tiers (after the user edits files on
// disk) so the catalog and prompt block reflect the change without a restart.
func (s *Server) handleReloadSkills(w http.ResponseWriter, r *http.Request) {
	store := ws(r).Runtime.Skills()
	store.Reload()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
