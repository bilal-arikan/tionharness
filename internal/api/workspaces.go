package api

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/proc"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// workspaceListItem is the switcher-facing view: registry metadata plus the
// visual identity (icon/color) from the workspace's own settings.
type workspaceListItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
	// Degraded marks a workspace that is registered but could not be opened; it has
	// no live handles, so the client must not switch into it — only rename/delete or
	// repair it on disk. DegradedReason carries the open failure.
	Degraded       bool   `json:"degraded,omitempty"`
	DegradedReason string `json:"degradedReason,omitempty"`
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	entries := s.workspaces.ListWithDegraded()
	out := make([]workspaceListItem, 0, len(entries))
	for _, m := range entries {
		item := workspaceListItem{
			ID: m.ID, Name: m.Name, CreatedAt: m.CreatedAt,
			Degraded: m.Degraded, DegradedReason: m.Reason,
		}
		if ws, err := s.workspaces.Get(m.ID); err == nil {
			cfg := ws.Settings()
			item.Icon = cfg.Icon
			item.Color = cfg.Color
		}
		out = append(out, item)
	}
	q := r.URL.Query()
	limit, offset, field, asc, listing, err := listQueryParams(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !listing {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if field != "" {
		// Workspaces carry no updated-at (only rename mutates one and it preserves
		// CreatedAt), so updated_* is a documented alias for created_*.
		less, err := tools.SortByField(field, asc,
			func(w workspaceListItem) int64 { return w.CreatedAt },
			func(w workspaceListItem) int64 { return w.CreatedAt },
			func(w workspaceListItem) string { return w.Name },
			func(w workspaceListItem) string { return w.ID })
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		slices.SortStableFunc(out, less)
	}
	page, total := tools.SlicePage(out, offset, limit)
	pageJSONResponse(w, page, total, offset, limit)
}

type createWorkspaceReq struct {
	Name string `json:"name"`
	// ProjectDir is an OPTIONAL working directory (cwd) new sessions in this
	// workspace start with — the project the workspace operates on. It is NOT the
	// workspace data dir: the data dir always uses the application default location.
	ProjectDir string `json:"projectDir"`
	Icon       string `json:"icon"`     // optional emoji identity
	Color      string `json:"color"`    // optional hex accent
	Template   string `json:"template"` // optional workspace template id (default "blank")

	// GitInit asks for `git init` (branch "main") in ProjectDir right after the
	// workspace is created — the common "new project folder" case, so the user does
	// not have to visit Workspace ▸ Proje afterwards. Ignored without a ProjectDir.
	// A missing folder is created first; a folder that is already a repo is left
	// untouched. Failure never fails the create: it is reported in gitInitError.
	GitInit bool `json:"gitInit"`
}

// createWorkspaceResp is the registry Meta (embedded, so the client's Workspace
// shape is unchanged) plus the outcome of the optional git init. The git step is
// advisory — the workspace exists either way — so its failure travels as a field
// rather than an HTTP error.
type createWorkspaceResp struct {
	workspace.Meta
	GitInit      bool   `json:"gitInit,omitempty"`
	GitInitError string `json:"gitInitError,omitempty"`
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[createWorkspaceReq](w, r)
	if !ok {
		return
	}
	// The data dir always uses the application default location — no per-workspace
	// data-folder override any more.
	wsNew, err := s.workspaces.Create(req.Name, "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// The store is open only now, so it missed the boot-time applySettings pass.
	if err := s.applyDebugJournalToStore(wsNew.DB); err != nil {
		s.logger.Error("apply debug journal policy failed", "workspace", wsNew.ID, "error", err)
	}

	// Apply the visual identity (icon/color) and the optional project directory.
	projectDir := strings.TrimSpace(req.ProjectDir)
	if req.Icon != "" || req.Color != "" || projectDir != "" {
		patch := workspace.WSSettingsPatch{}
		if req.Icon != "" {
			patch.Icon = &req.Icon
		}
		if req.Color != "" {
			patch.Color = &req.Color
		}
		if projectDir != "" {
			patch.DefaultWorkingDir = &projectDir
		}
		if _, err := s.workspaces.UpdateSettings(wsNew.ID, patch); err != nil {
			s.logger.Warn("apply workspace identity failed", "workspace", wsNew.ID, "error", err)
		}
	}

	// Seed the new workspace from the chosen market template (agents + flow +
	// schedules) so it is usable immediately. Unknown/empty template id falls back
	// to the embedded "blank" template.
	s.seedWorkspaceFromTemplate(r.Context(), wsNew, req.Template)

	resp := createWorkspaceResp{Meta: wsNew.Meta}
	if req.GitInit && projectDir != "" {
		// createDir=true: the create dialog explicitly offers "folder does not exist
		// yet — it will be created", so laying it out here is the intended behaviour.
		if err := prepareGitRepo(projectDir, true); err != nil {
			resp.GitInitError = err.Error()
			s.logger.Warn("workspace git init failed", "workspace", wsNew.ID, "dir", projectDir, "error", err)
		} else {
			resp.GitInit = true
		}
	}
	s.publishWorkspacesChanged("Yeni bir workspace oluşturuldu: " + wsNew.Name)
	writeJSON(w, http.StatusCreated, resp)
}

type attachWorkspaceReq struct {
	Path string `json:"path"` // absolute path of an existing workspace data dir
}

// handleAttachWorkspace registers an existing on-disk workspace data directory
// (chosen via the folder picker in first-run onboarding) as a workspace, without
// recreating its content. A folder that is not a valid workspace, or is already
// attached, yields 400 with a human message so the UI can show it inline.
func (s *Server) handleAttachWorkspace(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[attachWorkspaceReq](w, r)
	if !ok {
		return
	}
	wsNew, err := s.workspaces.Attach(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Same as create: the adopted store opens here, after the last applySettings.
	if err := s.applyDebugJournalToStore(wsNew.DB); err != nil {
		s.logger.Error("apply debug journal policy failed", "workspace", wsNew.ID, "error", err)
	}
	s.publishWorkspacesChanged("Bir workspace bağlandı: " + wsNew.Name)
	writeJSON(w, http.StatusCreated, wsNew.Meta)
}

// pickFolderReq/Resp carry the native folder-picker exchange.
type pickFolderResp struct {
	Path     string `json:"path"`
	Canceled bool   `json:"canceled"`
}

// handlePickFolder opens the OS native folder-selection dialog on the machine
// running the backend (a local desktop app) and returns the chosen path:
// PowerShell FolderBrowserDialog on Windows, AppleScript `choose folder` on
// macOS, zenity/kdialog on Linux. When no picker is available it returns an
// error so the UI falls back to manual path entry.
func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	cmd, err := folderPickerCommand(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "folder picker unavailable: "+err.Error())
		return
	}
	out, err := cmd.Output()
	if err != nil {
		// Every picker signals "user canceled" with exit status 1 (osascript also
		// prints error -128); treat that as a cancel, not a failure.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			writeJSON(w, http.StatusOK, pickFolderResp{Canceled: true})
			return
		}
		writeError(w, http.StatusInternalServerError, "folder picker unavailable: "+err.Error())
		return
	}
	path := strings.TrimSpace(string(out))
	if path != "" {
		// AppleScript's POSIX path of a folder ends with "/".
		path = filepath.Clean(path)
	}
	writeJSON(w, http.StatusOK, pickFolderResp{Path: path, Canceled: path == ""})
}

const pickFolderPrompt = "TionHarness workspace klasörü seç"

// folderPickerCommand builds the platform's native folder-dialog command. Its
// stdout is the chosen path; exit status 1 means the user canceled.
func folderPickerCommand(ctx context.Context) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "windows":
		const script = `Add-Type -AssemblyName System.Windows.Forms | Out-Null
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = '` + pickFolderPrompt + `'
$d.ShowNewFolderButton = $true
$top = New-Object System.Windows.Forms.Form
$top.TopMost = $true
if ($d.ShowDialog($top) -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.SelectedPath) }`
		// HideConsole (not proc.Command): suppress the PowerShell console flash but
		// keep the FolderBrowserDialog visible. proc.Command's HideWindow (SW_HIDE)
		// would hide the dialog too.
		cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script)
		proc.HideConsole(cmd)
		return cmd, nil
	case "darwin":
		// `activate` brings osascript's own dialog to the front without needing
		// Automation permission for another app.
		return exec.CommandContext(ctx, "osascript",
			"-e", "activate",
			"-e", `POSIX path of (choose folder with prompt "`+pickFolderPrompt+`")`), nil
	default:
		if p, err := exec.LookPath("zenity"); err == nil {
			return exec.CommandContext(ctx, p, "--file-selection", "--directory", "--title="+pickFolderPrompt), nil
		}
		if p, err := exec.LookPath("kdialog"); err == nil {
			return exec.CommandContext(ctx, p, "--getexistingdirectory", "", "--title", pickFolderPrompt), nil
		}
		return nil, errors.New("zenity or kdialog not found")
	}
}

func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.workspaces.Delete(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.publishWorkspacesChanged("Bir workspace silindi.")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}
