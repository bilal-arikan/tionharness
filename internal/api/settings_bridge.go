package api

import (
	"encoding/json"

	"github.com/bilal-arikan/tionswarm/internal/settings"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// settingsBridge adapts the application settings store + the server's live-apply
// hook to the tools.SettingsBridge interface, so the get_settings /
// update_settings agent tools can read and activate settings the same way the
// Settings screen does (persist to settings.json, then push into every live
// subsystem). The masked DTO is used everywhere — secret keys are never exposed.
type settingsBridge struct{ srv *Server }

// SettingsBridge returns this server's settings bridge for injection into the
// workspace runtimes (via Manager.SetSettingsBridge).
func (s *Server) SettingsBridge() tools.SettingsBridge { return settingsBridge{srv: s} }

func (b settingsBridge) Path() string { return b.srv.settings.Path() }

func (b settingsBridge) Snapshot() (string, error) {
	data, err := json.MarshalIndent(b.srv.settings.DTO(), "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b settingsBridge) Apply(patchJSON string) (string, error) {
	var patch settings.Patch
	if err := json.Unmarshal([]byte(patchJSON), &patch); err != nil {
		return "", err
	}
	if _, err := b.srv.settings.Apply(patch); err != nil {
		return "", err
	}
	// Push the new settings into the live subsystems, exactly like the HTTP
	// settings handler does after a UI edit.
	b.srv.applySettings()
	// Notify open UIs (over the /api/events SSE feed) so the Settings screen and
	// live theme refresh without a manual reload.
	b.srv.publishSettingsChanged("Bir ajan uygulama ayarlarını değiştirdi.")
	return b.Snapshot()
}
