package workspace

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// wsSettingsFile is the per-workspace settings document inside the workspace dir.
const wsSettingsFile = "ws-settings.json"

// defaultInstructions is the seed workspace prompt (workspace-specific system
// prompt addendum) a fresh workspace starts with. TionSwarm has no monolithic
// system prompt of its own — the workspace prompt IS the standing guidance every
// agent in the workspace carries — so this default gives new workspaces a full,
// TionSwarm-specific baseline instead of an empty prompt. A workspace whose
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
	Instructions    string `json:"instructions"`
	Icon            string `json:"icon"`  // emoji shown in the switcher/rail
	Color           string `json:"color"` // hex accent for visual identity
	PauseAutonomy   bool   `json:"pauseAutonomy"`

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

	// Cross-session awareness (workspace-specific) is always on: a short summary of
	// this workspace's recent past chat sessions is injected into an agent's context
	// on the session's first turn, and the list_sessions / archive_sessions /
	// conversation_search pull tools are always offered. Nothing is configurable.

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

	// AutoCaptureArtifacts toggles the turn-end trace scan that upserts every file
	// the agent wrote (Write/create_file) as an artifact automatically. Default OFF:
	// only files the agent DELIBERATELY registers via create_artifact become
	// artifacts — plain file writes (e.g. editing project source) stay off the
	// Artifacts screen. On: any file deliverable lands in the Artifacts screen
	// without an explicit call. The deliverable prompt guidance follows this toggle.
	AutoCaptureArtifacts bool `json:"autoCaptureArtifacts"`

	// BoardColumns overrides the default kanban column set for this workspace.
	// Empty/nil means "use db.DefaultBoardColumns()".
	BoardColumns []db.BoardColumnDef `json:"boardColumns,omitempty"`

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
	// consistent with TionSwarm's physical workspace isolation. Purely a client-side
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
		// Default OFF: writing a file no longer auto-registers an artifact. Only a
		// deliberate create_artifact call produces one, so editing project source
		// files does not pollute the Artifacts screen. Opt in per workspace.
		AutoCaptureArtifacts: false,
	}
}

// WSSettingsPatch is a partial update; nil fields are left unchanged. Name is
// handled separately (workspace rename) since it lives in the registry Meta.
type WSSettingsPatch struct {
	Name            *string `json:"name"`
	Instructions    *string `json:"instructions"`
	Icon            *string `json:"icon"`
	Color           *string `json:"color"`
	PauseAutonomy     *bool   `json:"pauseAutonomy"`
	DefaultWorkingDir *string `json:"defaultWorkingDir"`

	Theme       *string `json:"theme"`
	Accent      *string `json:"accent"`
	ThemePreset *string `json:"themePreset"`

	CodebaseMemoryEnabled  *bool   `json:"codebaseMemoryEnabled"`
	PromptEpochEnabled     *bool   `json:"promptEpochEnabled"`
	AutoCaptureArtifacts   *bool   `json:"autoCaptureArtifacts"`
	ShellOutputCompression *string `json:"shellOutputCompression"`

	BoardColumns *[]db.BoardColumnDef `json:"boardColumns"`

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
		w.Runtime.SetDefaultWorkDir(s.DefaultWorkingDir)
		w.Runtime.SetCodebaseMemory(s.CodebaseMemoryEnabled)
		w.Runtime.SetPromptEpoch(s.PromptEpochEnabled)
		w.Runtime.SetShellCompression(s.ShellOutputCompression)
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

	if patch.Name != nil {
		if err := m.Rename(id, *patch.Name); err != nil {
			return nil, err
		}
	}

	ws.settings.mu.Lock()
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
	if patch.DefaultWorkingDir != nil {
		ws.settings.cur.DefaultWorkingDir = *patch.DefaultWorkingDir
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
	if patch.AutoCaptureArtifacts != nil {
		ws.settings.cur.AutoCaptureArtifacts = *patch.AutoCaptureArtifacts
	}
	if patch.ShellOutputCompression != nil {
		ws.settings.cur.ShellOutputCompression = *patch.ShellOutputCompression
	}
	if patch.BoardColumns != nil {
		ws.settings.cur.BoardColumns = *patch.BoardColumns
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
	defaultWorkDir := ws.settings.cur.DefaultWorkingDir
	cbmEnabled := ws.settings.cur.CodebaseMemoryEnabled
	epochEnabled := ws.settings.cur.PromptEpochEnabled
	shellCompression := ws.settings.cur.ShellOutputCompression
	ws.settings.mu.Unlock()

	if err := ws.saveSettings(); err != nil {
		return nil, err
	}
	// Mirror an instructions change to config/instructions.md so the file and
	// ws-settings.json stay in sync (the file is the source of truth on reload).
	if patch.Instructions != nil {
		ws.writeInstructionsFile(instructions)
	}
	if ws.Runtime != nil {
		ws.Runtime.SetPaused(paused)
		ws.Runtime.SetInstructions(instructions)
		ws.Runtime.SetDefaultWorkDir(defaultWorkDir)
		ws.Runtime.SetCodebaseMemory(cbmEnabled)
		ws.Runtime.SetPromptEpoch(epochEnabled)
		ws.Runtime.SetShellCompression(shellCompression)
	}
	return ws, nil
}
