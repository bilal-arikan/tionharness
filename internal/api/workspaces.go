package api

import "net/http"

// workspaceListItem is the switcher-facing view: registry metadata plus the
// visual identity (icon/color) from the workspace's own settings.
type workspaceListItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, _ *http.Request) {
	metas := s.workspaces.List()
	out := make([]workspaceListItem, 0, len(metas))
	for _, m := range metas {
		item := workspaceListItem{ID: m.ID, Name: m.Name, CreatedAt: m.CreatedAt}
		if ws, err := s.workspaces.Get(m.ID); err == nil {
			cfg := ws.Settings()
			item.Icon = cfg.Icon
			item.Color = cfg.Color
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

type createWorkspaceReq struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req createWorkspaceReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	ws, err := s.workspaces.Create(req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ws.Meta)
}

func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.workspaces.Delete(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}
