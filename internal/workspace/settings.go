package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// wsSettingsFile is the per-workspace settings document inside the workspace dir.
const wsSettingsFile = "ws-settings.json"

// WSSettings holds the per-workspace overrides editable from the Settings
// screen's "Bu Workspace" category. Empty provider/model fall back to the
// application-global defaults; PauseAutonomy pauses only this workspace's
// runtime (heartbeat + scheduler), independent of the global brake.
// Instructions is free-form guidance injected into agents running in this
// workspace (workspace-specific system prompt addendum).
type WSSettings struct {
	Instructions    string `json:"instructions"`
	Icon            string `json:"icon"`  // emoji shown in the switcher/rail
	Color           string `json:"color"` // hex accent for visual identity
	DefaultProvider string `json:"defaultProvider"`
	DefaultModel    string `json:"defaultModel"`
	PauseAutonomy   bool   `json:"pauseAutonomy"`

	// Cross-session awareness (workspace-specific): inject a short summary of this
	// workspace's active + recent chat sessions into an agent's context, and offer
	// the list_sessions pull tool. Each workspace controls its own behaviour.
	SessionContextEnabled     bool `json:"sessionContextEnabled"`
	SessionContextEveryTurn   bool `json:"sessionContextEveryTurn"`   // false = only a session's first turn
	SessionContextRecentCount int  `json:"sessionContextRecentCount"` // past sessions listed (0 = default 5)
}

// defaultWSSettings is the seed used before overlaying a persisted ws-settings
// document, so a fresh workspace (or one whose file predates a new field) gets
// sensible defaults — notably cross-session awareness on, first-turn, 5 recent.
func defaultWSSettings() WSSettings {
	return WSSettings{
		SessionContextEnabled:     true,
		SessionContextEveryTurn:   false,
		SessionContextRecentCount: 5,
	}
}

// WSSettingsPatch is a partial update; nil fields are left unchanged. Name is
// handled separately (workspace rename) since it lives in the registry Meta.
type WSSettingsPatch struct {
	Name            *string `json:"name"`
	Instructions    *string `json:"instructions"`
	Icon            *string `json:"icon"`
	Color           *string `json:"color"`
	DefaultProvider *string `json:"defaultProvider"`
	DefaultModel    *string `json:"defaultModel"`
	PauseAutonomy   *bool   `json:"pauseAutonomy"`

	SessionContextEnabled     *bool `json:"sessionContextEnabled"`
	SessionContextEveryTurn   *bool `json:"sessionContextEveryTurn"`
	SessionContextRecentCount *int  `json:"sessionContextRecentCount"`
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
	if data, err := os.ReadFile(w.settingsPath()); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	w.settings.cur = s
	if w.Runtime != nil {
		w.Runtime.SetPaused(s.PauseAutonomy)
		w.Runtime.SetInstructions(s.Instructions)
		w.Runtime.SetSessionContext(s.SessionContextEnabled, s.SessionContextEveryTurn, s.SessionContextRecentCount)
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
	if patch.DefaultProvider != nil {
		ws.settings.cur.DefaultProvider = *patch.DefaultProvider
	}
	if patch.DefaultModel != nil {
		ws.settings.cur.DefaultModel = *patch.DefaultModel
	}
	if patch.PauseAutonomy != nil {
		ws.settings.cur.PauseAutonomy = *patch.PauseAutonomy
	}
	if patch.SessionContextEnabled != nil {
		ws.settings.cur.SessionContextEnabled = *patch.SessionContextEnabled
	}
	if patch.SessionContextEveryTurn != nil {
		ws.settings.cur.SessionContextEveryTurn = *patch.SessionContextEveryTurn
	}
	if patch.SessionContextRecentCount != nil {
		ws.settings.cur.SessionContextRecentCount = clampRecent(*patch.SessionContextRecentCount)
	}
	paused := ws.settings.cur.PauseAutonomy
	instructions := ws.settings.cur.Instructions
	scEnabled := ws.settings.cur.SessionContextEnabled
	scEvery := ws.settings.cur.SessionContextEveryTurn
	scRecent := ws.settings.cur.SessionContextRecentCount
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
		ws.Runtime.SetSessionContext(scEnabled, scEvery, scRecent)
	}
	return ws, nil
}

// clampRecent bounds the recent-session count to [1,20] to keep the prompt small.
func clampRecent(n int) int {
	if n < 1 {
		return 1
	}
	if n > 20 {
		return 20
	}
	return n
}
