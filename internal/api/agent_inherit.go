package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// writeAgentWriteError maps the inheritance/lock errors an agent write can
// return onto HTTP statuses, then falls back to writeDBError. Returns true when
// a response was written.
//
//	locked built-in          → 409 (derive a copy instead)
//	role already customised  → 409
//	bad parent / cycle       → 400
//	unknown override key     → 400
func writeAgentWriteError(w http.ResponseWriter, err error, notFoundMsg string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, db.ErrAgentLocked):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, db.ErrSystemRoleTaken):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, db.ErrAgentParentNotFound), errors.Is(err, db.ErrAgentParentCycle):
		writeError(w, http.StatusBadRequest, err.Error())
	case strings.HasPrefix(err.Error(), "unknown override field"):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		return writeDBError(w, err, notFoundMsg)
	}
	return true
}

type deriveAgentReq struct {
	// Name of the child; empty picks "<parent> (kopya)".
	Name string `json:"name"`
	// BindRole makes the child the workspace's customisation of the parent's
	// system role (see db.DeriveAgentOptions.BindRole).
	BindRole bool `json:"bindRole"`
}

// handleDeriveAgent creates a child that inherits every field from the target
// agent. With bindRole the child also takes over the parent's system role —
// the supported way to customise a built-in (locked) agent.
//
// The child is written to the REQUEST's workspace store, so a role
// customisation is workspace-local by construction: other workspaces still
// resolve the role from the built-in. Its default name says so
// (customizationName).
func (s *Server) handleDeriveAgent(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSONStrict[deriveAgentReq](w, r)
	if !ok {
		return
	}
	parentID := r.PathValue("id")
	wsp := ws(r)
	parent, err := wsp.DB.GetAgent(r.Context(), parentID)
	if writeDBError(w, err, "agent not found") {
		return
	}
	if parent.Deleted {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" && req.BindRole {
		name = customizationName(parent.Name, wsp.Name)
	}
	child, err := wsp.DB.DeriveAgent(r.Context(), parentID, db.DeriveAgentOptions{Name: name, BindRole: req.BindRole})
	if writeAgentWriteError(w, err, "agent not found") {
		return
	}
	s.logger.Info("agent derived", "parent", parent.ID, "child", child.ID, "name", child.Name, "bindRole", req.BindRole)
	writeJSON(w, http.StatusCreated, child)
}

// handleRestoreAgentDefaults makes a child inherit every field again (drops all
// overrides). A locked built-in has nothing to restore (409); a root agent has
// no parent to restore from (404).
func (s *Server) handleRestoreAgentDefaults(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	current, err := wsp.DB.GetAgent(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "agent not found") {
		return
	}
	if current.Locked {
		writeError(w, http.StatusConflict, db.ErrAgentLocked.Error())
		return
	}
	if current.ParentID == "" {
		writeError(w, http.StatusNotFound, "agent has no parent to restore from")
		return
	}
	restored, err := wsp.DB.ClearAgentOverrides(r.Context(), current.ID)
	if writeAgentWriteError(w, err, "agent not found") {
		return
	}
	s.logger.Info("agent overrides cleared", "agent", restored.Name, "id", restored.ID)
	writeJSON(w, http.StatusOK, restored)
}

// createDerivedAgent is the POST /api/agents branch for a request naming a
// parentId: derive the child, then pin the few fields the create form sends
// (a non-empty soul) as overrides. Everything else inherits.
func (s *Server) createDerivedAgent(w http.ResponseWriter, r *http.Request, req createAgentReq) {
	wsp := ws(r)
	child, err := wsp.DB.DeriveAgent(r.Context(), req.ParentID, db.DeriveAgentOptions{Name: req.Name})
	if writeAgentWriteError(w, err, "parent agent not found") {
		return
	}
	patch := db.AgentProfilePatch{}
	touched := false
	if strings.TrimSpace(req.Soul) != "" {
		patch.Soul = &req.Soul
		touched = true
	}
	if strings.TrimSpace(req.Identity) != "" {
		patch.Identity = &req.Identity
		touched = true
	}
	if req.Avatar != "" {
		patch.Avatar = &req.Avatar
		touched = true
	}
	if req.Color != "" {
		patch.Color = &req.Color
		touched = true
	}
	if touched {
		child, err = wsp.DB.UpdateAgent(r.Context(), child.ID, patch)
		if writeAgentWriteError(w, err, "agent not found") {
			return
		}
	}
	s.logger.Info("agent created (derived)", "agent", child.Name, "id", child.ID, "parent", req.ParentID)
	writeJSON(w, http.StatusCreated, child)
}
