package workspace

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// wsSettingsFile is the per-workspace settings document inside the workspace dir.
const wsSettingsFile = "ws-settings.json"

// defaultInstructions is the seed workspace prompt (workspace-specific system
// prompt addendum) a fresh workspace starts with. TionHarness has no monolithic
// system prompt of its own — the workspace prompt IS the standing guidance every
// agent in the workspace carries — so this default gives new workspaces a full,
// TionHarness-specific baseline instead of an empty prompt. A workspace whose
// ws-settings.json sets its own `instructions` overrides this seed.
//
//go:embed defaults/default-instructions.md
var defaultInstructions string

// WSSettings holds the per-workspace overrides editable from the Settings
// screen's "Bu Workspace" category. Empty provider/model fall back to the
// application-global defaults; PauseAutonomy pauses only this workspace's
// runtime (scheduler), independent of the global brake.
// Instructions is free-form guidance injected into agents running in this
// workspace (workspace-specific system prompt addendum).
type WSSettings struct {
	Instructions  string `json:"instructions"`
	Icon          string `json:"icon"`  // emoji shown in the switcher/rail
	Color         string `json:"color"` // hex accent for visual identity
	PauseAutonomy bool   `json:"pauseAutonomy"`

	// DefaultAgentId is the agent pre-selected for new sessions in this workspace.
	// Per-workspace (not global localStorage) so switching workspaces does not
	// silently overwrite another workspace's choice.
	DefaultAgentId string `json:"defaultAgentId"`

	// Per-workspace appearance overrides (client-side visual only). Empty fields
	// inherit the application-global appearance, so the UI re-themes itself when
	// the active workspace changes. Theme is the legacy base mode (dark/light/
	// system), ThemePreset a curated palette id, Accent a hex accent override.
	Theme       string `json:"theme"`
	Accent      string `json:"accent"`
	ThemePreset string `json:"themePreset"`

	// DefaultWorkingDir is the working directory (cwd) new sessions in this
	// workspace start with, for the built-in fs/shell tools. Empty = the physical
	// workspace dir. A session's own WorkingDir overrides it. the external agent project parity.
	DefaultWorkingDir string `json:"defaultWorkingDir"`

	// Card worktree lifecycle settings. Empty values use main and a sibling
	// .tionharness-worktrees/<workspace-id> directory respectively.
	WorktreeBaseRef string `json:"worktreeBaseRef,omitempty"`
	WorktreeRootDir string `json:"worktreeRootDir,omitempty"`

	// Cross-session awareness (workspace-specific) is always on: a short summary of
	// this workspace's recent past chat sessions is injected into an agent's context
	// on the session's first turn, and the list_sessions / archive_sessions /
	// conversation_search pull tools are always offered. Nothing is configurable.

	// TerseMode appends the workspace's terse ("caveman") reply-style prompt to
	// every agent's static system prefix. The text is the registry prompt "terse"
	// (workspace override <workspace>/config/prompts/terse.md → embedded default),
	// so it is editable per workspace like any other runtime prompt.
	//
	// Why a prompt and not a skill: a reply-style rule only works when it is always
	// in force. As a skill it sits in the Available Skills catalog as a one-line
	// summary and only reaches the model if the model itself decides to call
	// use_skill — which in practice means it rarely fires. Default off.
	TerseMode bool `json:"terseMode"`

	// CodebaseMemoryEnabled toggles the codebase-memory capability system for this
	// workspace: when a codebase-memory MCP server is present, inject a prompt hint,
	// route it at a per-workspace isolated store, auto-index the session cwd, and
	// offer the codebase_workspace_search tool. Default on; off = fully vanilla.
	CodebaseMemoryEnabled bool `json:"codebaseMemoryEnabled"`

	// PromptEpochEnabled toggles the prompt-epoch (frozen prompt-prefix snapshot)
	// system: a session's static system prompt + tool schemas freeze at session
	// start so mid-session config drift cannot bust the prompt cache; changes
	// adopt at compaction/idle/model-change or an explicit /refresh-context.
	// Default on; off = every turn recomposes from live state (pre-epoch behaviour).
	PromptEpochEnabled bool `json:"promptEpochEnabled"`

	// ShellOutputCompression overrides the in-process shell-output token-optimizer
	// (sqz) for this workspace: "" (or "auto") = follow sqz-hook detection (default),
	// "on" = force it on (needs the sqz binary), "off" = disable. See _Docs/17.
	ShellOutputCompression string `json:"shellOutputCompression,omitempty"`

	// ShellCommandRewrite overrides the in-process shell-COMMAND token-optimizer
	// (rtk) for this workspace: "" (or "auto") = follow rtk-hook detection (default),
	// "on" = force it on (needs the rtk binary), "off" = disable. Distinct from
	// ShellOutputCompression because the two act at opposite ends: rtk rewrites the
	// COMMAND before it runs (so a test runner reports only failures), sqz compresses
	// the OUTPUT after. They compose — see _Docs/17.
	ShellCommandRewrite string `json:"shellCommandRewrite,omitempty"`

	// BoardColumns overrides the default kanban column set for this workspace.
	// Empty/nil means "use db.DefaultBoardColumns()".
	BoardColumns []db.BoardColumnDef `json:"boardColumns,omitempty"`

	// BoardViews holds this workspace's user-created saved board views (named
	// filter + groupBy + sort presets). The built-in views (Tümü / Bugün / …) live
	// in the client and are never stored, so an empty list means "no custom views
	// yet". Which view is ACTIVE is deliberately NOT stored here: that is
	// per-window UI state (see _Docs/30-COKLU-PENCERE.md) and lives in the
	// client's localStorage, so two open windows can sit on different views
	// without overwriting each other.
	BoardViews []db.BoardViewDef `json:"boardViews,omitempty"`

	// IgnoredRecommendations holds the keys of post-create advisory cards
	// (WorkspaceRecommendations) the user dismissed for this workspace, so they are
	// not re-offered. Purely UI state — no runtime effect. Manageable (review +
	// un-ignore) from the Settings ▸ Öneriler panel.
	IgnoredRecommendations []string `json:"ignoredRecommendations,omitempty"`

	// DesktopNotifications overrides the app-global desktop-notification master
	// toggle (AppSettings.DesktopNotifications) for this workspace. A three-state
	// pointer: nil (absent in JSON) = inherit the global value; *true/*false =
	// force notifications on/off for this workspace regardless of the global
	// toggle. This lets a user silence a background "autonomous" workspace while
	// keeping notifications on for the one they actively work in (or vice versa),
	// consistent with TionHarness's physical workspace isolation. Purely a client-side
	// OS-toast gate — no backend runtime effect.
	DesktopNotifications *bool `json:"desktopNotifications,omitempty"`
}

// defaultWSSettings is the seed used before overlaying a persisted ws-settings
// document, so a fresh workspace (or one whose file predates a new field) gets
// sensible defaults.
func defaultWSSettings() WSSettings {
	return WSSettings{
		Instructions:          defaultInstructions,
		CodebaseMemoryEnabled: true,
		PromptEpochEnabled:    true,
	}
}

// WSSettingsPatch is a partial update; nil fields are left unchanged. Name is
// handled separately (workspace rename) since it lives in the registry Meta.
type WSSettingsPatch struct {
	Name              *string `json:"name"`
	Instructions      *string `json:"instructions"`
	Icon              *string `json:"icon"`
	Color             *string `json:"color"`
	PauseAutonomy     *bool   `json:"pauseAutonomy"`
	DefaultAgentId    *string `json:"defaultAgentId"`
	DefaultWorkingDir *string `json:"defaultWorkingDir"`
	WorktreeBaseRef   *string `json:"worktreeBaseRef"`
	WorktreeRootDir   *string `json:"worktreeRootDir"`

	Theme       *string `json:"theme"`
	Accent      *string `json:"accent"`
	ThemePreset *string `json:"themePreset"`

	TerseMode              *bool   `json:"terseMode"`
	CodebaseMemoryEnabled  *bool   `json:"codebaseMemoryEnabled"`
	PromptEpochEnabled     *bool   `json:"promptEpochEnabled"`
	ShellOutputCompression *string `json:"shellOutputCompression"`
	ShellCommandRewrite    *string `json:"shellCommandRewrite"`

	BoardColumns *[]db.BoardColumnDef `json:"boardColumns"`

	// BoardViews replaces the whole saved-view list (add/rename/delete are all
	// expressed as a full rewrite, matching how BoardColumns is edited).
	BoardViews *[]db.BoardViewDef `json:"boardViews"`

	IgnoredRecommendations *[]string `json:"ignoredRecommendations"`

	// DesktopNotifications is a pointer-of-pointer to distinguish all three states
	// the patch must express: nil = leave unchanged; non-nil pointing at nil (**bool
	// set, *bool nil) = clear the override (revert to "inherit global"); non-nil
	// pointing at a bool = force on/off for this workspace. It is populated from the
	// wire "inherit"|"on"|"off" string in UnmarshalJSON (json:"-" so the default
	// decoder does not touch it).
	DesktopNotifications **bool `json:"-"`
}

// UnmarshalJSON decodes the standard patch fields via a shadow type, then maps the
// three-state desktopNotifications wire string ("inherit"|"on"|"off") onto the
// DesktopNotifications **bool. An absent key leaves DesktopNotifications nil (patch
// unchanged); "inherit" sets it to a non-nil pointer to a nil *bool (clear the
// override); "on"/"off" set it to a pointer to true/false. An unrecognised value is
// rejected so a client typo cannot silently no-op.
func (p *WSSettingsPatch) UnmarshalJSON(data []byte) error {
	type alias WSSettingsPatch // shares the field set, drops the custom method to avoid recursion
	var shadow struct {
		alias
		DesktopNotifications *string `json:"desktopNotifications"`
	}
	if err := json.Unmarshal(data, &shadow); err != nil {
		return err
	}
	*p = WSSettingsPatch(shadow.alias)
	if shadow.DesktopNotifications != nil {
		switch *shadow.DesktopNotifications {
		case "inherit":
			var cleared *bool // nil inner pointer => clear the override
			p.DesktopNotifications = &cleared
		case "on":
			on := true
			onPtr := &on
			p.DesktopNotifications = &onPtr
		case "off":
			off := false
			offPtr := &off
			p.DesktopNotifications = &offPtr
		default:
			return errors.New("desktopNotifications must be one of: inherit, on, off")
		}
	}
	return nil
}

// ErrDefaultAgentSystem rejects making a built-in (system) agent the default
// agent for new sessions. System agents exist to serve the runtime — titling,
// compaction, worker profiles — and are not conversation partners, so they must
// never be pre-selected for a fresh chat. Their own runtime duties are
// unaffected. Callers may use errors.Is to map it to an API 400.
var ErrDefaultAgentSystem = errors.New("system agent cannot be the default agent for new sessions")

// checkDefaultAgent rejects a defaultAgentId pointing at a system agent. An id
// that resolves to nothing is left to the caller: a stale/unknown id is a
// separate concern (the client falls back to the first agent), and failing it
// here would break workspaces created before their agents exist.
func (w *Workspace) checkDefaultAgent(id string) error {
	if id == "" || w.DB == nil {
		return nil
	}
	agent, err := w.DB.GetAgent(context.Background(), id)
	if err != nil {
		return nil
	}
	if agent.System {
		return ErrDefaultAgentSystem
	}
	return nil
}

// sanitizeDefaultAgent repairs a persisted DefaultAgentId that points at a
// system agent — a value written before the write-path gate existed. It clears
// the setting (new sessions fall back to the first roster agent) and persists.
func (w *Workspace) sanitizeDefaultAgent(logger *slog.Logger) {
	w.settings.mu.RLock()
	id := w.settings.cur.DefaultAgentId
	w.settings.mu.RUnlock()
	if err := w.checkDefaultAgent(id); err == nil {
		return
	}
	w.settings.mu.Lock()
	w.settings.cur.DefaultAgentId = ""
	w.settings.mu.Unlock()
	if logger != nil {
		logger.Warn("cleared workspace default agent pointing at a system agent",
			"workspace", w.ID, "agent", id)
	}
	if err := w.saveSettings(); err != nil && logger != nil {
		logger.Warn("persist cleared default agent failed", "workspace", w.ID, "error", err)
	}
}

// settingsHolder is embedded in Workspace to guard concurrent settings access.
type settingsHolder struct {
	mu  sync.RWMutex
	cur WSSettings
}

// Settings returns a copy of this workspace's settings.
func (w *Workspace) Settings() WSSettings {
	w.settings.mu.RLock()
	defer w.settings.mu.RUnlock()
	return w.settings.cur
}

// settingsPath is the on-disk location of this workspace's settings.
func (w *Workspace) settingsPath() string {
	return filepath.Join(w.DataDir, wsSettingsFile)
}

// loadSettings reads ws-settings.json (absent = zero-value defaults) and applies
// the autonomy pause to the live runtime.
func (w *Workspace) loadSettings() {
	// Seed defaults first so an absent file — or a file written before a field
	// existed — yields the intended defaults rather than zero values.
	s := defaultWSSettings()
	legacy := false
	if data, err := os.ReadFile(w.settingsPath()); err == nil {
		_ = json.Unmarshal(data, &s)
		// One-time migration: the abstract per-workspace "defaultProvider/defaultModel"
		// override was removed (provider/model is now agent-based). Strip the dead keys
		// from any pre-existing file by rewriting it clean below.
		legacy = bytes.Contains(data, []byte(`"defaultProvider"`)) || bytes.Contains(data, []byte(`"defaultModel"`))
	}
	w.settings.cur = s
	if legacy {
		_ = w.saveSettings()
	}
	if w.Runtime != nil {
		w.Runtime.SetPaused(s.PauseAutonomy)
		w.Runtime.SetInstructions(s.Instructions)
		w.Runtime.SetTerseMode(s.TerseMode)
		w.Runtime.SetDefaultWorkDir(s.DefaultWorkingDir)
		w.Runtime.SetCodebaseMemory(s.CodebaseMemoryEnabled)
		w.Runtime.SetPromptEpoch(s.PromptEpochEnabled)
		w.Runtime.SetShellCompression(s.ShellOutputCompression)
		w.Runtime.SetShellCommandRewrite(s.ShellCommandRewrite)
	}
}

// saveSettings persists the current settings atomically (temp + rename).
func (w *Workspace) saveSettings() error {
	w.settings.mu.RLock()
	data, err := json.MarshalIndent(w.settings.cur, "", "  ")
	w.settings.mu.RUnlock()
	if err != nil {
		return err
	}
	tmp := w.settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, w.settingsPath())
}

// Rename changes a workspace's display name and persists the registry.
func (m *Manager) Rename(id, name string) error {
	if name == "" {
		return nil
	}
	m.mu.Lock()
	ws, ok := m.workspaces[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("workspace not found")
	}
	ws.Meta.Name = name
	m.mu.Unlock()
	return m.persist()
}

// UpdateSettings applies a settings patch to a workspace, persists it, and
// re-applies the autonomy pause to the live runtime.
func (m *Manager) UpdateSettings(id string, patch WSSettingsPatch) (*Workspace, error) {
	ws, err := m.Get(id)
	if err != nil {
		return nil, err
	}

	// Reject a malformed saved-view list BEFORE any part of the patch is applied,
	// so a bad view cannot land half a settings update on disk.
	if patch.BoardViews != nil {
		if err := db.ValidateBoardViews(*patch.BoardViews); err != nil {
			return nil, err
		}
	}

	// Same rule: reject before anything is applied, so a system agent cannot land
	// half a settings update on disk. This is the single write path for
	// DefaultAgentId, so the gate covers the HTTP handler and every internal
	// caller (template/market install, workspace bridge) alike.
	if patch.DefaultAgentId != nil {
		if err := ws.checkDefaultAgent(*patch.DefaultAgentId); err != nil {
			return nil, err
		}
	}

	if patch.Name != nil {
		if err := m.Rename(id, *patch.Name); err != nil {
			return nil, err
		}
	}

	ws.settings.mu.Lock()
	// Captured before BoardColumns is overwritten below, so the rename/delete
	// diff run after saveSettings has both the pre- and post-patch column set.
	var oldBoardCols, newBoardCols []db.BoardColumnDef
	if patch.BoardColumns != nil {
		oldBoardCols = ws.settings.cur.BoardColumns
		if len(oldBoardCols) == 0 {
			oldBoardCols = db.DefaultBoardColumns()
		}
		newBoardCols = *patch.BoardColumns
	}
	if patch.Instructions != nil {
		ws.settings.cur.Instructions = *patch.Instructions
	}
	if patch.Icon != nil {
		ws.settings.cur.Icon = *patch.Icon
	}
	if patch.Color != nil {
		ws.settings.cur.Color = *patch.Color
	}
	if patch.PauseAutonomy != nil {
		ws.settings.cur.PauseAutonomy = *patch.PauseAutonomy
	}
	if patch.DefaultAgentId != nil {
		ws.settings.cur.DefaultAgentId = *patch.DefaultAgentId
	}
	if patch.TerseMode != nil {
		ws.settings.cur.TerseMode = *patch.TerseMode
	}
	if patch.DefaultWorkingDir != nil {
		ws.settings.cur.DefaultWorkingDir = *patch.DefaultWorkingDir
	}
	if patch.WorktreeBaseRef != nil {
		ws.settings.cur.WorktreeBaseRef = *patch.WorktreeBaseRef
	}
	if patch.WorktreeRootDir != nil {
		ws.settings.cur.WorktreeRootDir = *patch.WorktreeRootDir
	}
	if patch.Theme != nil {
		ws.settings.cur.Theme = *patch.Theme
	}
	if patch.Accent != nil {
		ws.settings.cur.Accent = *patch.Accent
	}
	if patch.ThemePreset != nil {
		ws.settings.cur.ThemePreset = *patch.ThemePreset
	}
	if patch.CodebaseMemoryEnabled != nil {
		ws.settings.cur.CodebaseMemoryEnabled = *patch.CodebaseMemoryEnabled
	}
	if patch.PromptEpochEnabled != nil {
		ws.settings.cur.PromptEpochEnabled = *patch.PromptEpochEnabled
	}
	if patch.ShellOutputCompression != nil {
		ws.settings.cur.ShellOutputCompression = *patch.ShellOutputCompression
	}
	if patch.ShellCommandRewrite != nil {
		ws.settings.cur.ShellCommandRewrite = *patch.ShellCommandRewrite
	}
	if patch.BoardColumns != nil {
		ws.settings.cur.BoardColumns = *patch.BoardColumns
	}
	if patch.BoardViews != nil {
		ws.settings.cur.BoardViews = *patch.BoardViews
	}
	if patch.IgnoredRecommendations != nil {
		ws.settings.cur.IgnoredRecommendations = *patch.IgnoredRecommendations
	}
	if patch.DesktopNotifications != nil {
		// *patch is the new override state: nil (inner) clears the override so this
		// workspace inherits the global toggle again; a non-nil *bool forces it.
		ws.settings.cur.DesktopNotifications = *patch.DesktopNotifications
	}
	paused := ws.settings.cur.PauseAutonomy
	instructions := ws.settings.cur.Instructions
	terseMode := ws.settings.cur.TerseMode
	defaultWorkDir := ws.settings.cur.DefaultWorkingDir
	cbmEnabled := ws.settings.cur.CodebaseMemoryEnabled
	epochEnabled := ws.settings.cur.PromptEpochEnabled
	shellCompression := ws.settings.cur.ShellOutputCompression
	shellRewrite := ws.settings.cur.ShellCommandRewrite
	ws.settings.mu.Unlock()

	if err := ws.saveSettings(); err != nil {
		return nil, err
	}
	// A column rename or delete leaves tasks pointing at a BoardState the new
	// column set no longer has, which would silently drop them off the board.
	// Migrate them now that the new column set is durably saved.
	if patch.BoardColumns != nil && ws.DB != nil {
		if _, err := ws.DB.MigrateBoardColumns(context.Background(), oldBoardCols, newBoardCols); err != nil {
			return nil, err
		}
	}
	// Mirror an instructions change to config/instructions.md so the file and
	// ws-settings.json stay in sync (the file is the source of truth on reload).
	if patch.Instructions != nil {
		ws.writeInstructionsFile(instructions)
	}
	if ws.Runtime != nil {
		ws.Runtime.SetPaused(paused)
		ws.Runtime.SetInstructions(instructions)
		ws.Runtime.SetTerseMode(terseMode)
		ws.Runtime.SetDefaultWorkDir(defaultWorkDir)
		ws.Runtime.SetCodebaseMemory(cbmEnabled)
		ws.Runtime.SetPromptEpoch(epochEnabled)
		ws.Runtime.SetShellCompression(shellCompression)
		ws.Runtime.SetShellCommandRewrite(shellRewrite)
	}
	return ws, nil
}
