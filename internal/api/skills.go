package api

import (
	"net/http"
	"os/exec"
	"path/filepath"

	"github.com/bilal/swarmgo/internal/skills"
)

// skillDetail is a skill plus its (lazily read) markdown body, returned by the
// detail endpoint so the Skills screen can render full instructions on demand.
type skillDetail struct {
	skills.Skill
	Body string `json:"body"`
	// Dir is the folder containing this skill's SKILL.md, surfaced so the UI can
	// copy the path (Skill.Path itself stays json:"-"). Empty if unknown.
	Dir string `json:"dir"`
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
	dir := ""
	if sk.Path != "" {
		dir = filepath.Dir(sk.Path)
	}
	writeJSON(w, http.StatusOK, skillDetail{Skill: sk, Body: body, Dir: dir})
}

// skillInputReq is the create/update payload from the Skills editor.
type skillInputReq struct {
	Slug        string `json:"slug,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	WhenToUse   string `json:"whenToUse"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	Shared      bool   `json:"shared"`
	Body        string `json:"body"`
}

func (req skillInputReq) input() skills.SkillInput {
	return skills.SkillInput{
		Name:        req.Name,
		Description: req.Description,
		WhenToUse:   req.WhenToUse,
		Icon:        req.Icon,
		Color:       req.Color,
		Shared:      req.Shared,
		Body:        req.Body,
	}
}

// skillDetailFor builds the detail DTO (skill + body + dir) for a freshly
// created/updated skill, so the UI can render it without a second round-trip.
func skillDetailFor(store *skills.Store, sk skills.Skill) skillDetail {
	body, _ := store.Body(sk.Slug)
	dir := ""
	if sk.Path != "" {
		dir = filepath.Dir(sk.Path)
	}
	return skillDetail{Skill: sk, Body: body, Dir: dir}
}

// handleCreateSkill creates a new skill (folder + SKILL.md) in the workspace
// tier and returns it with its body.
func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	var req skillInputReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	store := ws(r).Runtime.Skills()
	sk, err := store.Create(req.Slug, req.input())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, skillDetailFor(store, sk))
}

// handleUpdateSkill rewrites an existing skill's frontmatter + body in place.
func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	var req skillInputReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	store := ws(r).Runtime.Skills()
	sk, err := store.Update(r.PathValue("slug"), req.input())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, skillDetailFor(store, sk))
}

// handleDeleteSkill removes a skill's folder from disk and strips the slug from
// every agent that referenced it, so no agent keeps a dangling skill reference.
func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	slug := r.PathValue("slug")
	if err := wsp.Runtime.Skills().Delete(slug); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := wsp.DB.RemoveSkillFromAgents(r.Context(), slug); err != nil {
		s.logger.Warn("strip deleted skill from agents failed", "slug", slug, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRevealSkill opens the skill's folder in the OS file manager on the
// machine running the backend (local desktop app). Windows: Explorer.
func (s *Server) handleRevealSkill(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	sk, ok := ws(r).Runtime.Skills().Get(slug)
	if !ok || sk.Path == "" {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	dir := filepath.Dir(sk.Path)
	if err := exec.CommandContext(r.Context(), "explorer.exe", dir).Start(); err != nil {
		// explorer.exe returns a non-zero exit code even on success; only a
		// failure to *start* the process is a real error.
		s.logger.Warn("reveal skill folder failed", "slug", slug, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": dir})
}

// handleSetSkillAccess flips a skill between shared (on-demand) and restricted
// by rewriting its SKILL.md frontmatter, then returns the updated skill.
func (s *Server) handleSetSkillAccess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Shared bool `json:"shared"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	sk, err := ws(r).Runtime.Skills().SetAccess(r.PathValue("slug"), req.Shared)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

// handleSetSkillAutoSummary toggles whether a skill's summary is auto-injected
// into every agent's prompt, by rewriting its SKILL.md frontmatter, then returns
// the updated skill.
func (s *Server) handleSetSkillAutoSummary(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AutoSummary bool `json:"autoSummary"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	sk, err := ws(r).Runtime.Skills().SetAutoSummary(r.PathValue("slug"), req.AutoSummary)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

// handleReloadSkills re-scans the skill tiers (after the user edits files on
// disk) so the catalog and prompt block reflect the change without a restart.
func (s *Server) handleReloadSkills(w http.ResponseWriter, r *http.Request) {
	store := ws(r).Runtime.Skills()
	store.Reload()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
