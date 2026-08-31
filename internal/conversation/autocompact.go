package conversation

// Auto-compaction strategy. Picks WHAT the manager does when automatic context
// compaction fires: the built-in rolling-summary fold, the CLI provider's own
// native compaction, or "auto" (native when the provider can and a warm CLI
// session is live, rolling otherwise). Mirrors settings.AutoCompactMode; the
// value is carried here so the runtime can read it without importing settings.
const (
	AutoCompactRolling = "rolling"
	AutoCompactNative  = "native"
	AutoCompactAuto    = "auto"
)

// SetAutoCompactMode updates the strategy live from the Settings screen. An
// unrecognised value is ignored so a malformed push can never wedge compaction
// into a mode the manager does not implement.
func (m *Manager) SetAutoCompactMode(mode string) {
	switch mode {
	case AutoCompactRolling, AutoCompactNative, AutoCompactAuto:
	default:
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoCompactMode = mode
}

// AutoCompactMode reports the current strategy, defaulting to rolling for a
// manager built before the mode was ever pushed.
func (m *Manager) AutoCompactMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.autoCompactMode == "" {
		return AutoCompactRolling
	}
	return m.autoCompactMode
}
