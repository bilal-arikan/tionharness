package api

import (
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/skills"
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
	Group       string `json:"group"`
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
		Group:       req.Group,
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
	req, ok := bindJSON[skillInputReq](w, r)
	if !ok {
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

// importSkillReq is the import payload. source "local" reads a skill directory
// from disk (path); source "github" fetches a github.com tree/blob URL. (SK-IMP)
type importSkillReq struct {
	Source string `json:"source"` // "local" (default) | "github"
	Path   string `json:"path"`   // local skill directory (source=local)
	URL    string `json:"url"`    // github skill URL (source=github)
	Slug   string `json:"slug"`   // optional slug override
	Shared bool   `json:"shared"` // advertise as on-demand (default restricted)
}

// handleImportSkill imports a Claude Code skill into the workspace tier, mapping
// its frontmatter (allowed-tools→always_allow, paths, provenance…) and copying its
// bundled files. Returns the ImportResult (mapped fields + warnings) plus the
// created skill detail. (SK-IMP)
func (s *Server) handleImportSkill(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[importSkillReq](w, r)
	if !ok {
		return
	}
	source := req.Source
	if source == "" {
		source = "local"
	}
	location := req.Path
	if source == "github" {
		location = req.URL
	}
	if strings.TrimSpace(location) == "" {
		writeError(w, http.StatusBadRequest, "path (local) or url (github) is required")
		return
	}
	store := ws(r).Runtime.Skills()
	sk, res, err := store.ImportFromSource(source, location, req.Slug, req.Shared)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"result": res, "skill": skillDetailFor(store, sk)})
}

// handleUpdateSkill rewrites an existing skill's frontmatter + body in place.
func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[skillInputReq](w, r)
	if !ok {
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
	// Detached from r.Context() so it isn't killed when the handler returns.
	if err := exec.Command("explorer.exe", dir).Start(); err != nil {
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

// handleSetSkillNameOnly toggles whether a skill is advertised as slug-only
// (description + when-to-use suppressed) in the Available Skills block, by
// rewriting its SKILL.md frontmatter, then returns the updated skill.
func (s *Server) handleSetSkillNameOnly(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NameOnly bool `json:"nameOnly"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	sk, err := ws(r).Runtime.Skills().SetNameOnly(r.PathValue("slug"), req.NameOnly)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

// handleSetSkillVisibility forces a skill into one of the four visibility tiers
// (full | summary | name-only | hidden) — the skill analogue of a tool's
// visibility — by rewriting its SKILL.md frontmatter flags together, then returns
// the updated skill. This is the single entry point the Skills screen's 4-way
// selector drives.
func (s *Server) handleSetSkillVisibility(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Visibility string `json:"visibility"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	sk, err := ws(r).Runtime.Skills().SetVisibility(r.PathValue("slug"), req.Visibility)
	if err != nil {
		// An invalid tier is a client error; a missing skill is a 404. Distinguish
		// by message prefix so a typo returns 400, not 404.
		if strings.HasPrefix(err.Error(), "invalid visibility tier") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

// handleSetSkillGroup rewrites a skill's `group` frontmatter (its Skills-UI
// organisation bucket) without touching any other field, then returns the updated
// skill. An empty group ungroups the skill. This is the per-skill endpoint the
// Skills screen's bulk "set group" action calls for each selected skill.
func (s *Server) handleSetSkillGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Group string `json:"group"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	sk, err := ws(r).Runtime.Skills().SetGroup(r.PathValue("slug"), req.Group)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

// handleRestoreSkill overwrites a shipped skill's SKILL.md with its embedded
// default, discarding local changes, then reloads the catalog. The deliberate
// counterpart to the automatic re-seed, which only touches files it can PROVE are
// untouched prior ships (internal/seed) — an edited skill, or one seeded before
// the shipped-hash ledger existed, stays frozen until asked from here.
//
// Only the GLOBAL tier has shipped defaults. A workspace-tier skill with the same
// slug is a deliberate override in a different file: restoring "its" default would
// silently write to a file the user is not looking at, so it is refused.
func (s *Server) handleRestoreSkill(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	store := ws(r).Runtime.Skills()
	sk, ok := store.Get(slug)
	if !ok {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	if sk.Source != skills.SourceGlobal || !skills.HasDefault(slug) {
		writeError(w, http.StatusNotFound, "skill has no shipped default")
		return
	}
	// <globalDir>/<slug>/SKILL.md → <globalDir>. Derived from the resolved skill
	// rather than re-deriving the data dir, so the write lands in the very tier the
	// catalog resolved this skill from.
	globalDir := filepath.Dir(filepath.Dir(sk.Path))
	if err := skills.RestoreDefault(globalDir, slug); writeDBError(w, err, "") {
		return
	}
	store.Reload()
	restored, ok := store.Get(slug)
	if !ok {
		writeError(w, http.StatusInternalServerError, "restored skill failed to reload")
		return
	}
	s.logger.Info("skill restored to default", "skill", slug)
	writeJSON(w, http.StatusOK, restored)
}

// handleReloadSkills re-scans the skill tiers (after the user edits files on
// disk) so the catalog and prompt block reflect the change without a restart.
func (s *Server) handleReloadSkills(w http.ResponseWriter, r *http.Request) {
	store := ws(r).Runtime.Skills()
	store.Reload()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
