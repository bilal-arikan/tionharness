package agent

import "sync"

// Default journal bounds, used when the settings-driven values are unset (0).
const (
	DefaultJournalCap           = 50   // newest journal entries kept per agent
	DefaultJournalMaxLen        = 1024 // max runes stored per journal entry
	DefaultAutoReflectThreshold = 30   // journal count that triggers auto-reflect
)

// Tunables holds process-wide, settings-driven knobs that cut across every
// workspace runtime: the global autonomy pause switch and an optional model
// override for auto-title generation. A single instance is created at boot and
// shared (by pointer) with every Runtime and the API server, so a settings
// change applies uniformly regardless of which workspace runtime reads it.
type Tunables struct {
	mu            sync.RWMutex
	pauseAutonomy bool
	titleModel    string
	shellEnabled  bool // gates the high-risk built-in `shell` tool (off by default)
	selfManage    bool // gates the self-management tool suite (off by default)
	journalCap    int  // 0 → DefaultJournalCap
	journalMaxLen int  // 0 → DefaultJournalMaxLen

	autoReflect          bool // run the dream cycle automatically as journals grow
	autoReflectThreshold int  // 0 → DefaultAutoReflectThreshold
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

// SetShellEnabled toggles the built-in `shell` tool. It is off by default
// because it grants arbitrary command execution inside the workspace sandbox;
// enable it only once a permission/approval layer is in place.
func (t *Tunables) SetShellEnabled(enabled bool) {
	t.mu.Lock()
	t.shellEnabled = enabled
	t.mu.Unlock()
}

// ShellEnabled reports whether the built-in `shell` tool may be offered.
func (t *Tunables) ShellEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.shellEnabled
}

// SetSelfManageEnabled toggles the self-management tool suite (create/edit/
// delete agents, flows, schedules, artifacts; add memories; read logs). Off by
// default: the suite roughly doubles the tool catalog (token cost per turn) and
// lets agents alter the workspace, so it is opt-in per workspace settings.
func (t *Tunables) SetSelfManageEnabled(enabled bool) {
	t.mu.Lock()
	t.selfManage = enabled
	t.mu.Unlock()
}

// SelfManageEnabled reports whether the self-management tool suite may be offered.
func (t *Tunables) SelfManageEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.selfManage
}

// SetJournalLimits sets the journal ring-buffer cap (max entries kept per agent)
// and the per-entry length cap. A value of 0 selects the built-in default.
func (t *Tunables) SetJournalLimits(cap, maxLen int) {
	t.mu.Lock()
	t.journalCap = cap
	t.journalMaxLen = maxLen
	t.mu.Unlock()
}

// JournalCap returns how many journal entries an agent keeps (default when unset).
func (t *Tunables) JournalCap() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.journalCap <= 0 {
		return DefaultJournalCap
	}
	return t.journalCap
}

// JournalMaxLen returns the per-entry journal length cap in runes (default when unset).
func (t *Tunables) JournalMaxLen() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.journalMaxLen <= 0 {
		return DefaultJournalMaxLen
	}
	return t.journalMaxLen
}

// SetAutoReflect configures the automatic dream cycle: whether it runs and the
// journal count that triggers it (0 threshold selects the built-in default).
func (t *Tunables) SetAutoReflect(enabled bool, threshold int) {
	t.mu.Lock()
	t.autoReflect = enabled
	t.autoReflectThreshold = threshold
	t.mu.Unlock()
}

// AutoReflect reports whether auto-reflect is enabled.
func (t *Tunables) AutoReflect() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.autoReflect
}

// AutoReflectThreshold returns the journal count that triggers auto-reflect.
func (t *Tunables) AutoReflectThreshold() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.autoReflectThreshold <= 0 {
		return DefaultAutoReflectThreshold
	}
	return t.autoReflectThreshold
}
