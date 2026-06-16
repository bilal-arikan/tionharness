package api

import (
	"net/http"
	"os/exec"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/workspace"
)

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
	Name  string `json:"name"`
	Path  string `json:"path"`  // optional parent folder for the workspace data dir
	Icon  string `json:"icon"`  // optional emoji identity
	Color string `json:"color"` // optional hex accent
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req createWorkspaceReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	wsNew, err := s.workspaces.Create(req.Name, strings.TrimSpace(req.Path))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Apply the visual identity (icon/color) when provided.
	if req.Icon != "" || req.Color != "" {
		patch := workspace.WSSettingsPatch{}
		if req.Icon != "" {
			patch.Icon = &req.Icon
		}
		if req.Color != "" {
			patch.Color = &req.Color
		}
		if _, err := s.workspaces.UpdateSettings(wsNew.ID, patch); err != nil {
			s.logger.Warn("apply workspace identity failed", "workspace", wsNew.ID, "error", err)
		}
	}

	// Seed the new workspace with a default agent so it is usable immediately.
	s.seedDefaultAgent(r, wsNew)

	writeJSON(w, http.StatusCreated, wsNew.Meta)
}

// seedDefaultAgent creates a starter agent in a freshly created workspace using
// the configured provider/model defaults (workspace override → app default →
// claude-cli last resort), so the roster is never empty.
func (s *Server) seedDefaultAgent(r *http.Request, wsNew *workspace.Workspace) {
	cfg := s.settings.Get()
	wsCfg := wsNew.Settings()

	provider := wsCfg.DefaultProvider
	if provider == "" {
		provider = cfg.DefaultProvider
	}
	if provider == "" {
		provider = "claude-cli"
	}
	model := wsCfg.DefaultModel
	if model == "" {
		model = cfg.DefaultModel
	}

	if _, err := wsNew.DB.CreateAgent(r.Context(), db.Agent{
		Name:     "Asistan",
		Soul:     "You are a helpful assistant.",
		Provider: provider,
		Model:    model,
	}); err != nil {
		s.logger.Warn("seed default agent failed", "workspace", wsNew.ID, "error", err)
	}
}

// pickFolderReq/Resp carry the native folder-picker exchange.
type pickFolderResp struct {
	Path     string `json:"path"`
	Canceled bool   `json:"canceled"`
}

// handlePickFolder opens the OS native folder-selection dialog on the machine
// running the backend (a local desktop app) and returns the chosen path. On
// Windows it uses a PowerShell FolderBrowserDialog; on other platforms it
// returns an error so the UI falls back to manual path entry.
func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	const script = `Add-Type -AssemblyName System.Windows.Forms | Out-Null
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'SwarmGo workspace klasörü seç'
$d.ShowNewFolderButton = $true
$top = New-Object System.Windows.Forms.Form
$top.TopMost = $true
if ($d.ShowDialog($top) -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.SelectedPath) }`

	cmd := exec.CommandContext(r.Context(), "powershell.exe", "-NoProfile", "-STA", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "folder picker unavailable: "+err.Error())
		return
	}
	path := strings.TrimSpace(string(out))
	writeJSON(w, http.StatusOK, pickFolderResp{Path: path, Canceled: path == ""})
}

func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.workspaces.Delete(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}
