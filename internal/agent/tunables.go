package agent

import "sync"

// Tunables holds process-wide, settings-driven knobs that cut across every
// workspace runtime: the global autonomy pause switch and an optional model
// override for auto-title generation. A single instance is created at boot and
// shared (by pointer) with every Runtime and the API server, so a settings
// change applies uniformly regardless of which workspace runtime reads it.
type Tunables struct {
	mu            sync.RWMutex
	pauseAutonomy bool
	titleModel    string
}

// NewTunables constructs an empty (unpaused, no title override) Tunables.
func NewTunables() *Tunables { return &Tunables{} }

// SetAutonomyPaused toggles the global autonomy brake. When paused, autonomous
// provider calls (heartbeat, scheduler) are rejected before reaching a model;
// manual chat and run-now are unaffected.
func (t *Tunables) SetAutonomyPaused(paused bool) {
	t.mu.Lock()
	t.pauseAutonomy = paused
	t.mu.Unlock()
}

// AutonomyPaused reports the current global pause state.
func (t *Tunables) AutonomyPaused() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.pauseAutonomy
}

// SetTitleModel sets the model used for auto-title generation. Empty means use
// the titling agent's own model.
func (t *Tunables) SetTitleModel(model string) {
	t.mu.Lock()
	t.titleModel = model
	t.mu.Unlock()
}

// TitleModel returns the configured title-model override ("" = agent default).
func (t *Tunables) TitleModel() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.titleModel
}
