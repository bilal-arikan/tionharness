package agent

import "sync"

// Default journal bounds, used when the settings-driven values are unset (0).
const (
	DefaultJournalCap           = 50   // newest journal entries kept per agent
	DefaultJournalMaxLen        = 1024 // max runes stored per journal entry
	DefaultAutoReflectThreshold = 30   // journal count that triggers auto-reflect
	DefaultSessionContextRecent = 5    // past sessions listed in the cross-session block
)

// Default turn-recovery (A1) bounds, applied to a freshly constructed Tunables so
// test runtimes (which never call applySettings) get production-sane behaviour.
const (
	DefaultMaxTokenRetries    = 3 // resume attempts after the output-token cap
	DefaultReactiveKeepRecent = 6 // in-flight messages kept verbatim when compacting
)

// Default spawn guards. They bound the fire-and-forget spawn_session surface so a
// burst of spawns can neither pin unbounded goroutines nor fan a single turn out
// into a spawn storm.
const (
	DefaultSpawnMaxConcurrent = 16 // max simultaneously-running spawned sessions
	DefaultSpawnMaxPerTurn    = 4  // max spawns one agent turn may launch
)

// Default tool-output compaction bounds. System A (deterministic) trims every
// tool result; System B (LLM intent-aware summary) only fires past its byte
// threshold. Both default to sane values used by test runtimes.
const (
	DefaultCompactMaxLines     = 200   // System A: lines kept before middle elision
	DefaultCompactMaxBytes     = 12288 // System A: hard byte cap after line work
	DefaultCompactLLMThreshold = 8192  // System B: only summarize output larger than this
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
	delegation    bool // gates the agent→agent `call_agent` tool (off by default)
	delegMaxDepth int  // 0 → DefaultMaxDelegationDepth
	delegMaxCalls int  // 0 → DefaultMaxDelegationCalls

	spawnMaxConcurrent int // 0 → DefaultSpawnMaxConcurrent
	spawnMaxPerTurn    int // 0 → DefaultSpawnMaxPerTurn
	journalCap    int  // 0 → DefaultJournalCap
	journalMaxLen int  // 0 → DefaultJournalMaxLen

	autoReflect          bool // run the dream cycle automatically as journals grow
	autoReflectThreshold int  // 0 → DefaultAutoReflectThreshold

	// Turn recovery (A1) — structural handling of output-token cutoffs and
	// context overflow inside the native agentic tool loop.
	reactiveCompact    bool // fold older in-flight history + retry on context overflow
	maxTokenRetries    int  // resume attempts after the output cap (0 = disabled)
	reactiveKeepRecent int  // messages kept verbatim when compacting (<2 → default)

	// Tool-output token optimization — two independent, parallel systems.
	// System A: deterministic compaction (free, rule-based, every result).
	compactDeterministic bool // master switch for System A
	compactMaxLines      int  // 0 → DefaultCompactMaxLines
	compactMaxBytes      int  // 0 → DefaultCompactMaxBytes
	// System B: LLM intent-aware summary (costs a cheap model call, gated by size).
	compactLLM          bool   // master switch for System B
	compactLLMThreshold int    // 0 → DefaultCompactLLMThreshold
	compactModel        string // model id for System B; "" → titleModel, then agent's own model
}

// NewTunables constructs a Tunables with the recovery knobs at their built-in
// defaults (the other knobs default to their zero value = off/unset). Production
// overrides everything from settings via the Set* methods; tests that skip
// applySettings still get sane recovery behaviour.
func NewTunables() *Tunables {
	return &Tunables{
		reactiveCompact:    true,
		maxTokenRetries:    DefaultMaxTokenRetries,
		reactiveKeepRecent: DefaultReactiveKeepRecent,
	}
}

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

// SetDelegationEnabled toggles the agent→agent `call_agent` tool. Off by
// default: delegation multiplies token cost (each call runs another full agent
// turn) and lets a single turn fan out across agents. The runner still enforces
// depth, cycle and per-turn call-budget guards when enabled.
func (t *Tunables) SetDelegationEnabled(enabled bool) {
	t.mu.Lock()
	t.delegation = enabled
	t.mu.Unlock()
}

// DelegationEnabled reports whether the call_agent tool may be offered.
func (t *Tunables) DelegationEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.delegation
}

// SetDelegationLimits sets the per-turn delegation guards: max nesting depth and
// max total delegations per turn. A value of 0 selects the built-in default.
func (t *Tunables) SetDelegationLimits(maxDepth, maxCalls int) {
	t.mu.Lock()
	t.delegMaxDepth = maxDepth
	t.delegMaxCalls = maxCalls
	t.mu.Unlock()
}

// DelegationMaxDepth returns the max delegation nesting depth (default when unset).
func (t *Tunables) DelegationMaxDepth() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.delegMaxDepth <= 0 {
		return DefaultMaxDelegationDepth
	}
	return t.delegMaxDepth
}

// DelegationMaxCalls returns the max delegations per turn (default when unset).
func (t *Tunables) DelegationMaxCalls() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.delegMaxCalls <= 0 {
		return DefaultMaxDelegationCalls
	}
	return t.delegMaxCalls
}

// SetSpawnLimits sets the fire-and-forget spawn guards: the max number of
// simultaneously-running spawned sessions and the max spawns a single agent turn
// may launch. A value of 0 selects the built-in default.
func (t *Tunables) SetSpawnLimits(maxConcurrent, maxPerTurn int) {
	t.mu.Lock()
	t.spawnMaxConcurrent = maxConcurrent
	t.spawnMaxPerTurn = maxPerTurn
	t.mu.Unlock()
}

// SpawnMaxConcurrent returns the cap on simultaneously-running spawned sessions
// (default when unset).
func (t *Tunables) SpawnMaxConcurrent() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.spawnMaxConcurrent <= 0 {
		return DefaultSpawnMaxConcurrent
	}
	return t.spawnMaxConcurrent
}

// SpawnMaxPerTurn returns the cap on spawns launched by a single agent turn
// (default when unset).
func (t *Tunables) SpawnMaxPerTurn() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.spawnMaxPerTurn <= 0 {
		return DefaultSpawnMaxPerTurn
	}
	return t.spawnMaxPerTurn
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

// SetRecoveryLimits configures the A1 turn-recovery knobs: whether reactive
// compaction runs on context overflow, how many times a turn may resume after
// the output-token cap (0 disables resume), and how many in-flight messages a
// reactive compaction keeps verbatim.
func (t *Tunables) SetRecoveryLimits(reactiveCompact bool, maxTokenRetries, keepRecent int) {
	t.mu.Lock()
	t.reactiveCompact = reactiveCompact
	t.maxTokenRetries = maxTokenRetries
	t.reactiveKeepRecent = keepRecent
	t.mu.Unlock()
}

// ReactiveCompact reports whether context-overflow compaction-and-retry is on.
func (t *Tunables) ReactiveCompact() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.reactiveCompact
}

// MaxTokenRetries returns the output-cap resume budget. A returned 0 means resume
// is disabled (a capped answer is surfaced as-is), which is a valid setting.
func (t *Tunables) MaxTokenRetries() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.maxTokenRetries < 0 {
		return 0
	}
	return t.maxTokenRetries
}

// ReactiveKeepRecent returns the in-flight compaction tail size (default when <2,
// since a smaller tail cannot guarantee a safe fold boundary).
func (t *Tunables) ReactiveKeepRecent() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.reactiveKeepRecent < 2 {
		return DefaultReactiveKeepRecent
	}
	return t.reactiveKeepRecent
}

// SetToolCompaction configures the two independent tool-output optimization
// systems. System A (deterministic) and System B (LLM summary) are toggled
// separately and may run in parallel (A first, then B on whatever remains over
// its threshold). A value of 0 selects the built-in default for each limit.
func (t *Tunables) SetToolCompaction(deterministic bool, maxLines, maxBytes int, llm bool, llmThreshold int, model string) {
	t.mu.Lock()
	t.compactDeterministic = deterministic
	t.compactMaxLines = maxLines
	t.compactMaxBytes = maxBytes
	t.compactLLM = llm
	t.compactLLMThreshold = llmThreshold
	t.compactModel = model
	t.mu.Unlock()
}

// CompactDeterministic reports whether System A (deterministic compaction) is on.
func (t *Tunables) CompactDeterministic() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.compactDeterministic
}

// CompactMaxLines returns System A's line cap before middle elision (default when unset).
func (t *Tunables) CompactMaxLines() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.compactMaxLines <= 0 {
		return DefaultCompactMaxLines
	}
	return t.compactMaxLines
}

// CompactMaxBytes returns System A's hard byte cap (default when unset).
func (t *Tunables) CompactMaxBytes() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.compactMaxBytes <= 0 {
		return DefaultCompactMaxBytes
	}
	return t.compactMaxBytes
}

// CompactLLM reports whether System B (LLM intent-aware summary) is on.
func (t *Tunables) CompactLLM() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.compactLLM
}

// CompactLLMThreshold returns the byte size above which System B summarizes a
// tool result (default when unset).
func (t *Tunables) CompactLLMThreshold() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.compactLLMThreshold <= 0 {
		return DefaultCompactLLMThreshold
	}
	return t.compactLLMThreshold
}

// CompactModel returns the dedicated model id for System B's summary, or "" when
// unset (callers then fall back to the title model, then the agent's own model).
func (t *Tunables) CompactModel() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.compactModel
}
