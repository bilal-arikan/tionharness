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
	data, err := os.ReadFile(w.settingsPath())
	if err == nil {
		var s WSSettings
		if json.Unmarshal(data, &s) == nil {
			w.settings.cur = s
		}
	}
	if w.Runtime != nil {
		w.Runtime.SetPaused(w.settings.cur.PauseAutonomy)
		w.Runtime.SetInstructions(w.settings.cur.Instructions)
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
	paused := ws.settings.cur.PauseAutonomy
	instructions := ws.settings.cur.Instructions
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
	}
	return ws, nil
}
