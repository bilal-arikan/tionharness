package api

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// workspaceSettingsDTO is the client view of a workspace's editable settings,
// combining its registry name with its per-workspace overrides and live stats.
type workspaceSettingsDTO struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Instructions      string `json:"instructions"`
	Icon              string `json:"icon"`
	Color             string `json:"color"`
	PauseAutonomy     bool   `json:"pauseAutonomy"`
	DefaultWorkingDir string `json:"defaultWorkingDir"`
	CreatedAt         int64  `json:"createdAt"`

	// ClaudeHomeDir is THIS workspace's resolved claude-cli config home
	// (<workspace>/claude-home), exported into the CLI subprocess as
	// CLAUDE_CONFIG_DIR. Read-only/informational: it is derived from the workspace
	// root, not user-editable. Surfaced so the Settings screen can show the real
	// per-workspace path instead of the app-global fallback (which is identical for
	// every workspace). Mirrors agent.workspaceClaudeHomeDir / EnsureWorkspaceClaudeHome.
	ClaudeHomeDir string `json:"claudeHomeDir"`

	// Per-workspace appearance overrides (empty = inherit global).
	Theme       string `json:"theme"`
	Accent      string `json:"accent"`
	ThemePreset string `json:"themePreset"`

	// TerseMode appends the workspace's terse ("caveman") reply-style prompt (the
	// registry prompt "terse") to every agent's static system prefix.
	TerseMode bool `json:"terseMode"`

	// CodebaseMemoryEnabled toggles the codebase-memory capability system (hint +
	// isolated store + auto-index + codebase_workspace_search) for this workspace.
	CodebaseMemoryEnabled bool `json:"codebaseMemoryEnabled"`

	// PromptEpochEnabled toggles the prompt-epoch (frozen prompt-prefix snapshot)
	// system for this workspace (see promptepoch.go).
	PromptEpochEnabled bool `json:"promptEpochEnabled"`

	// AutoCaptureArtifacts toggles turn-end auto-capture of written files as
	// artifacts (see artifacts_auto.go). Off = only deliberate create_artifact
	// calls produce artifacts.
	AutoCaptureArtifacts bool `json:"autoCaptureArtifacts"`

	// ShellOutputCompression is the in-process shell-output token-optimizer
	// override: "" (auto — follow the sqz-hook opt-in), "on" or "off". Must be
	// echoed back, otherwise the settings selector always renders "auto" and a
	// workspace that really is forced on reads as unconfigured.
	ShellOutputCompression string `json:"shellOutputCompression"`

	// ShellCommandRewrite is the in-process shell-COMMAND token-optimizer (rtk)
	// override: "" (auto — follow the rtk-hook opt-in), "on" or "off". Echoed back
	// for the same reason as ShellOutputCompression.
	ShellCommandRewrite string `json:"shellCommandRewrite"`

	// BoardColumns is the ordered column set for this workspace's kanban board.
	// Always non-nil: falls back to db.DefaultBoardColumns() when unconfigured.
	BoardColumns []db.BoardColumnDef `json:"boardColumns"`

	// BoardViews lists this workspace's user-created saved board views (named
	// filter + groupBy + sort presets). Always non-nil so the client can render an
	// empty menu section cleanly; the built-in views are defined client-side.
	BoardViews []db.BoardViewDef `json:"boardViews"`

	// IgnoredRecommendations lists the dismissed advisory-card keys for this
	// workspace (always non-nil so the client can render an empty list cleanly).
	IgnoredRecommendations []string `json:"ignoredRecommendations"`

	// DesktopNotifications is this workspace's override of the app-global
	// desktop-notification master toggle, exposed as a three-state string so the
	// client can distinguish "not set" from an explicit off:
	//   "inherit" → follow AppSettings.DesktopNotifications (default)
	//   "on"      → force OS toasts on for this workspace
	//   "off"     → force OS toasts off for this workspace
	DesktopNotifications string `json:"desktopNotifications"`

	AgentCount   int `json:"agentCount"`
	SessionCount int `json:"sessionCount"`
	TaskCount    int `json:"taskCount"`
}

// desktopNotificationsToString renders the workspace's three-state
// desktop-notification override (*bool) as a stable string for the DTO:
// nil → "inherit", true → "on", false → "off".
func desktopNotificationsToString(v *bool) string {
	if v == nil {
		return "inherit"
	}
	if *v {
		return "on"
	}
	return "off"
}

func toWorkspaceSettingsDTO(ctx context.Context, w *workspace.Workspace) workspaceSettingsDTO {
	s := w.Settings()
	cols := s.BoardColumns
	if len(cols) == 0 {
		cols = db.DefaultBoardColumns()
	}
	dto := workspaceSettingsDTO{
		ID:                w.ID,
		Name:              w.Name,
		Instructions:      s.Instructions,
		Icon:              s.Icon,
		Color:             s.Color,
		PauseAutonomy:     s.PauseAutonomy,
		DefaultWorkingDir: s.DefaultWorkingDir,
		CreatedAt:         w.CreatedAt,

		// <workspace>/claude-home — DataDir is the workspace root (see workspace
		// manager: EnsureWorkspaceClaudeHome(dir) with the same join).
		ClaudeHomeDir: filepath.Join(w.DataDir, "claude-home"),

		Theme:       s.Theme,
		Accent:      s.Accent,
		ThemePreset: s.ThemePreset,

		CodebaseMemoryEnabled: s.CodebaseMemoryEnabled,
		PromptEpochEnabled:    s.PromptEpochEnabled,
		AutoCaptureArtifacts:  s.AutoCaptureArtifacts,

		ShellOutputCompression: s.ShellOutputCompression,
		ShellCommandRewrite:    s.ShellCommandRewrite,

		BoardColumns: cols,

		TerseMode: s.TerseMode,

		// Non-nil for a clean empty array in JSON (nil marshals to null).
		BoardViews:             append([]db.BoardViewDef{}, s.BoardViews...),
		IgnoredRecommendations: append([]string{}, s.IgnoredRecommendations...),

		DesktopNotifications: desktopNotificationsToString(s.DesktopNotifications),
	}
	if agents, err := w.DB.ListAgents(ctx); err == nil {
		dto.AgentCount = len(agents)
	}
	if sessions, err := w.DB.ListSessions(ctx, ""); err == nil {
		dto.SessionCount = len(sessions)
	}
	if tasks, err := w.DB.ListTasks(ctx); err == nil {
		dto.TaskCount = len(tasks)
	}
	return dto
}

// handleGetWorkspaceSettings returns the active workspace's settings.
func (s *Server) handleGetWorkspaceSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toWorkspaceSettingsDTO(r.Context(), ws(r)))
}

// claudeAuthDTO reports whether THIS workspace's claude-home is authenticated,
// as measured by a live pre-flight probe.
type claudeAuthDTO struct {
	LoggedIn bool `json:"loggedIn"`
	// Installed reports whether the claude CLI binary is resolvable at all. It
	// separates "CLI missing" (steer the user to the Providers screen) from "CLI
	// present but not logged in" (offer the auth popup). Only meaningful when
	// LoggedIn is false.
	Installed     bool   `json:"installed"`
	ClaudeHomeDir string `json:"claudeHomeDir"`
	Detail        string `json:"detail,omitempty"` // failure reason when LoggedIn is false
}

// handleWorkspaceClaudeAuth runs a lightweight, tool-free pre-flight probe against
// this workspace's claude-home and reports whether the claude-cli is logged in. It
// spawns a minimal `claude -p` (a few hundred ms), so it is ON-DEMAND only — the
// Settings screen calls it behind a "Verify login" action, never on every load — and
// lets the user catch an auth lapse before an agent turn burns on it.
func (s *Server) handleWorkspaceClaudeAuth(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	home := filepath.Join(wsp.DataDir, "claude-home")
	p, err := s.providers.Get("claude-cli")
	if err != nil {
		writeJSON(w, http.StatusOK, claudeAuthDTO{ClaudeHomeDir: home, Detail: err.Error()})
		return
	}
	cli, ok := p.(*providers.ClaudeCLI)
	if !ok {
		writeError(w, http.StatusInternalServerError, "claude-cli provider unavailable")
		return
	}
	// The CLI binary must exist before a login probe makes sense. When it is
	// missing there is nothing to authenticate — report Installed:false so the
	// client can steer the user to set up a provider instead of showing a login
	// popup for a CLI that cannot run.
	if !cli.Installed() {
		writeJSON(w, http.StatusOK, claudeAuthDTO{ClaudeHomeDir: home,
			Detail: "claude CLI not found (set its path in Providers, or use an API-key provider)"})
		return
	}
	cli.SetConfigDir(home)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if perr := cli.ProbeAuth(ctx); perr != nil {
		writeJSON(w, http.StatusOK, claudeAuthDTO{Installed: true, ClaudeHomeDir: home, Detail: perr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, claudeAuthDTO{LoggedIn: true, Installed: true, ClaudeHomeDir: home})
}

// handleUpdateWorkspaceSettings applies a partial update (including rename) to
// the active workspace and persists it.
func (s *Server) handleUpdateWorkspaceSettings(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	patch, ok := bindJSON[workspace.WSSettingsPatch](w, r)
	if !ok {
		return
	}
	// Saved-view shape is client-authored, so a malformed one is a 400, not a 500.
	// UpdateSettings validates again for non-HTTP callers; this check exists only
	// to pick the right status code.
	if patch.BoardViews != nil {
		if err := db.ValidateBoardViews(*patch.BoardViews); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	updated, err := s.workspaces.UpdateSettings(wsp.ID, patch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Board-column shape (keys / labels / colors) lives in workspace settings,
	// so a save here is the only way those changes reach the Network screen's
	// live-mode column anchors. Publishing a board event — the same channel
	// task CRUD uses — lets every open window refresh without polling and
	// covers cross-window sync (where window.dispatchEvent never reaches).
	if patch.BoardColumns != nil {
		publishEntityChange(wsp, "board", "Boards sütunları güncellendi", "",
			map[string]string{"view": "board", "op": "columns_changed"})
	}
	// Saved views live in the same settings document, so the same board channel
	// carries them: every open window re-pulls settings and picks up a view the
	// user saved (or deleted) elsewhere.
	if patch.BoardViews != nil {
		publishEntityChange(wsp, "board", "Board görünümleri güncellendi", "",
			map[string]string{"view": "board", "op": "views_changed"})
	}
	writeJSON(w, http.StatusOK, toWorkspaceSettingsDTO(r.Context(), updated))
}
