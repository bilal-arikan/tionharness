package agent

import "sync"

// DefaultSessionContextRecent bounds the past sessions listed in the
// cross-session context block.
const DefaultSessionContextRecent = 5

// Default turn-recovery (A1) bounds, applied to a freshly constructed Tunables so
// test runtimes (which never call applySettings) get production-sane behaviour.
const (
	DefaultMaxTokenRetries    = 3 // resume attempts after the output-token cap
	DefaultReactiveKeepRecent = 6 // in-flight messages kept verbatim when compacting
	DefaultProviderRetryMax   = 2 // retries per turn for transient provider faults (429/5xx/timeout)
)

// Default spawn guards. They bound the fire-and-forget spawn_session surface so a
// burst of spawns can neither pin unbounded goroutines nor fan a single turn out
// into a spawn storm.
const (
	DefaultSpawnMaxConcurrent = 16 // max simultaneously-running spawned sessions
	DefaultSpawnMaxPerTurn    = 4  // max spawns one agent turn may launch
)

// Default coordinator/worker guards (see internal/agent/coordination.go). They
// bound the M2 coordination loop so a coordinator can neither fan out unbounded
// workers nor spin forever on worker → notify → new-turn feedback.
const (
	DefaultCoordinatorMaxWorkers = 8  // max active workers a single coordinator may run at once
	DefaultCoordinatorMaxTurns   = 50 // max auto-triggered coordinator turns per session (notify-loop cap)
)

// Default tool-output compaction bounds. System A (deterministic) trims every
// tool result; System B (LLM intent-aware summary) only fires past its byte
// threshold. Both default to sane values used by test runtimes.
const (
	DefaultCompactMaxLines     = 200   // System A: lines kept before middle elision
	DefaultCompactMaxBytes     = 16384 // System A: hard byte cap (above B's threshold so A never pre-empts B)
	DefaultCompactLLMThreshold = 12288 // System B: only summarize output larger than this (~the external agent project's 12K)
)

// Tunables holds process-wide, settings-driven knobs that cut across every
// workspace runtime: an optional model override for auto-title generation and
// context/journal/memory controls. (Autonomy pausing is per-workspace — see
// Runtime.SetPaused — so it is not a process-wide knob.) A single instance is created at boot and
// shared (by pointer) with every Runtime and the API server, so a settings
// change applies uniformly regardless of which workspace runtime reads it.
type Tunables struct {
	mu               sync.RWMutex
	titleModel       string
	shellEnabled     bool // gates the high-risk built-in `shell` tool (off by default)
	cliHooks         bool // pass PreToolUse/PostToolUse hooks to claude-cli via --settings (on by default)
	cliPersist       bool // keep a long-lived claude-cli process per session (on by default)
	cliSysPromptFile bool // hand claude-cli system prompt via temp file vs inline (off by default = inline)
	delegMaxDepth    int  // 0 → DefaultMaxDelegationDepth
	delegMaxCalls    int  // 0 → DefaultMaxDelegationCalls

	spawnMaxConcurrent int // 0 → DefaultSpawnMaxConcurrent
	spawnMaxPerTurn    int // 0 → DefaultSpawnMaxPerTurn
	coordMaxWorkers    int // 0 → DefaultCoordinatorMaxWorkers
	coordMaxTurns      int // 0 → DefaultCoordinatorMaxTurns

	// Turn recovery (A1) — structural handling of output-token cutoffs and
	// context overflow inside the native agentic tool loop.
	reactiveCompact    bool // fold older in-flight history + retry on context overflow
	maxTokenRetries    int  // resume attempts after the output cap (0 = disabled)
	reactiveKeepRecent int  // messages kept verbatim when compacting (<2 → default)
	maxOutputTokens    int  // generation cap override (0 = auto: per-model family)
	providerRetryMax   int  // transient provider-fault retries per turn (<0 → default, 0 = disabled)

	// Tool-loop guardrail (self-healing Faz B): per-turn loop detection over
	// tool calls. Warnings append recovery guidance to failing results (on by
	// default); the hard stop additionally blocks/halts past the upper
	// thresholds (opt-in circuit breaker).
	toolGuardWarnings bool
	toolGuardHardStop bool

	// Tool-output token optimization — two independent, parallel systems.
	// System A: deterministic compaction (free, rule-based, every result).
	compactDeterministic bool // master switch for System A
	compactMaxLines      int  // 0 → DefaultCompactMaxLines
	compactMaxBytes      int  // 0 → DefaultCompactMaxBytes
	// System B: LLM intent-aware summary (costs a cheap model call, gated by size).
	compactLLM          bool   // master switch for System B
	compactLLMThreshold int    // 0 → DefaultCompactLLMThreshold
	compactModel        string // model id for System B; "" → titleModel, then agent's own model
	// contextBudgetTokens mirrors settings.MaxContextTokens so the byte thresholds
	// above scale with the budget (CG-9): bigger budget → bigger tool results
	// tolerated before compaction. 0 → default budget (scale 1).
	contextBudgetTokens int

	// Working-directory guards (fs/shell are otherwise unconfined).
	autonomousConfine    bool // confine fs/shell to the working dir on autonomous turns (default on)
	gitWorktreeIsolation bool // give autonomous sessions a per-session git worktree (default off)

	// Autonomous boot/verification sequence (Anthropic long-running-agent harness
	// discipline). When on, headless turns get a short reminder to orient → recall
	// → select one task → verify the baseline → work → close the loop before acting.
	autonomousBootSeq bool // inject the boot-sequence reminder on autonomous turns (default on)

	// Native tool-loop iteration cap (applied each iteration, so settings changes
	// take effect on the next turn without restart). <0 → defaultMaxToolIters;
	// 0 → unlimited (only recovery steps may terminate the loop); >0 → cap.
	maxToolIters int

	// Context reset / handoff (Anthropic "harness design" pattern). When a long
	// autonomous turn runs up against the context limit, in-place compaction alone
	// leaves "context anxiety"; instead the runtime can write a handoff artifact and
	// spawn a FRESH session to continue in a clean window.
	handoffAuto      bool    // auto-reset after an autonomous turn that hit the context limit (default off)
	handoffPressure  float64 // context-fill ratio above which auto-reset is allowed (0 → DefaultHandoffPressure)
	handoffMaxChain  int     // max reset-chain depth before falling back to plain compaction (0 → DefaultHandoffMaxChain)
	handoffWriteFile bool    // also write the handoff to <workdir>/.tionswarm/handoff.md (default off)

	// Persistent progress (Anthropic claude-progress convention). When on, the
	// todo_write checklist is persisted to <cwd>/.tionswarm/progress.json so it
	// survives across sessions; a fresh session reads it back at start.
	progressPersist bool // persist the checklist to disk (default on)
	progressResume  bool // inject a resumed-progress block on a fresh session (default on)

	// Autonomous self-completion: when an autonomous turn (scheduler/spawn/wake)
	// ends with unfinished work (open todos, or a trailing lazy-tool activation
	// whose tools only take effect next turn), the runtime auto-issues a
	// continuation turn — up to autoContinueMax times — so unattended work
	// finishes instead of stalling. Budget-gated; stops on no tool progress.
	autoContinue    bool // enable autonomous auto-continue (default on)
	autoContinueMax int  // 0 → DefaultAutoContinueMax

	// Per-session debug journal (parallel observability stream). When on, the
	// runtime appends structured events (turn timings, llm-call token spend, tool
	// latency/size, hook decisions, errors, compaction, recovery) to each
	// session's debug.jsonl, readable by the agent (read_session_debug) and the
	// UI for optimisation + self-improvement. debugJournalCap bounds the file.
	debugJournal    bool // emit the parallel debug stream (default on)
	debugJournalCap int  // newest events kept per session (0 = default)

	// fileFreshnessGuard (Claude Code parity) — when true, the built-in Edit and
	// Write tools enforce a read-before-write / not-modified-since-read check: an
	// Edit (or an overwrite of an existing file) errors unless the file was read
	// this session and is unchanged since, so an out-of-band edit is never silently
	// clobbered. Default on. See internal/tools/readtracker.go.
	fileFreshnessGuard bool

	// autoTagSessions — when true, the runtime derives well-known session tags from
	// turn outcomes + session state (tool-error/error/goal/goal-done/archived) so an
	// automation can scan + repair them. Default on. See internal/agent/autotag.go.
	autoTagSessions bool

	// codeMode (_Docs/44) — when true (and the shell gate is on), the MCP
	// catalog is additionally exposed as generated Python bindings behind the
	// run_code tool (code execution with MCP: schemas stay out of context,
	// intermediate data stays in the execution environment). Settings-driven
	// (enableCodeMode, default off); TIONSWARM_CODE_MODE=1 seeds the setting at
	// boot. See codemode_tunable.go.
	codeMode bool

	// autonomousTaskBudget (beta) — when > 0, autonomous turns announce this
	// token budget to the model via the API-native task-budget directive
	// (output_config.task_budget): the model sees a running countdown for the
	// whole agentic loop and paces itself, wrapping up gracefully instead of
	// being cut off by the iteration cap. A soft, model-aware complement to the
	// hard guards (maxToolIters/auto-continue caps); only adaptive-class
	// anthropic models honour it. 0 = off (default).
	autonomousTaskBudget int

	// nativeToolSearch (beta) — when true, native anthropic tool turns ship the
	// FULL tool catalog with lazy tools marked defer_loading plus the server-side
	// tool-search tool: the model discovers tools by regex without an
	// activate_tools round-trip, discovered schemas are APPENDED (cache-safe),
	// and the shipped tools block is byte-stable across loop iterations.
	// TionSwarm's own activation builtins keep working alongside. Off by default.
	nativeToolSearch bool

	// programmaticTools — when true, native anthropic tool turns add the
	// code-execution server tool and mark eligible builtins code-callable
	// (allowed_callers): the model invokes tools from Python inside Anthropic's
	// container, keeping intermediate results out of context. The API-native
	// counterpart of the local run_code code mode (which needs local Python);
	// MCP and interactive tools are excluded. Off by default.
	programmaticTools bool

	// webTools — when true, native anthropic tool turns add the server-side
	// web search + web fetch tools: searches run on Anthropic's infrastructure
	// and return cited results in the same response. Billed per search
	// (conservative per-turn max_uses ceilings applied). Off by default.
	webTools bool

	// serverCompaction mirrors the AnthropicServerCompaction beta so the tool
	// loop knows to echo assistant content verbatim (compaction blocks must ride
	// back exactly). The beta itself is applied provider-side (WithBetas).
	serverCompaction bool
}

// DefaultDebugJournalCap mirrors db.DefaultDebugJournalCap as the resolved
// default when no explicit cap is configured.
const DefaultDebugJournalCap = 5000

// DefaultAutoContinueMax bounds how many extra turns an autonomous run will
// auto-issue to finish work the agent left pending, when no explicit max is set.
const DefaultAutoContinueMax = 10

// Default context-reset / handoff bounds.
const (
	DefaultHandoffPressure = 0.90 // auto-reset only well above the memory-pressure warning (0.70)
	DefaultHandoffMaxChain = 20   // cap consecutive context resets so a loop can't chain forever
)

// NewTunables constructs a Tunables with the recovery knobs at their built-in
// defaults (the other knobs default to their zero value = off/unset). Production
// overrides everything from settings via the Set* methods; tests that skip
// applySettings still get sane recovery behaviour.
func NewTunables() *Tunables {
	return &Tunables{
		reactiveCompact:    true,
		maxTokenRetries:    DefaultMaxTokenRetries,
		reactiveKeepRecent: DefaultReactiveKeepRecent,
		providerRetryMax:   DefaultProviderRetryMax,
		// Guardrail warnings on by default (gentle nudge appended to failing
		// results); the hard stop stays opt-in from settings.
		toolGuardWarnings: true,
		// Autonomous turns (no human in the loop) re-confine fs/shell to the working
		// dir by default — the safety brake for the otherwise-unconfined tools.
		autonomousConfine: true,
		// Boot-sequence reminder on by default: a few tokens per headless turn buys
		// orient → verify-baseline discipline. Production overrides from settings.
		autonomousBootSeq: true,
		// Persistent progress on by default: it only adds a per-project file and is
		// transparent to existing behaviour. Production overrides from settings.
		progressPersist: true,
		progressResume:  true,
		// Autonomous self-completion on by default (production overrides from
		// settings via SetAutoContinue): an unattended turn that stalls after tool
		// activation or with open todos continues itself, bounded at 10 turns.
		autoContinue:    true,
		autoContinueMax: DefaultAutoContinueMax,
		// Debug journal on by default: it only adds a per-session file and is
		// transparent to existing behaviour. Production overrides from settings.
		debugJournal:    true,
		debugJournalCap: DefaultDebugJournalCap,
		// File freshness guard on by default (Claude Code parity): Edit/Write refuse to
		// clobber a file changed out-of-band since it was last read. Production overrides
		// from settings via SetFileFreshnessGuard.
		fileFreshnessGuard: true,
		// Auto-tagging on by default (production overrides from settings via
		// SetAutoTagSessions); test runtimes that skip applySettings still auto-tag.
		autoTagSessions: true,
		maxToolIters:    -1,
	}
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

// SetCLIHooksEnabled toggles whether the workspace's PreToolUse/PostToolUse hooks
// are passed to claude-cli agents (via the generated --settings file). On by
// default; turn off to keep hooks native-only when a hook authored for TionSwarm's
// shell misbehaves under the CLI's own hook runner.
func (t *Tunables) SetCLIHooksEnabled(enabled bool) {
	t.mu.Lock()
	t.cliHooks = enabled
	t.mu.Unlock()
}

// CLIHooksEnabled reports whether hooks are passed through to claude-cli agents.
func (t *Tunables) CLIHooksEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cliHooks
}

// SetClaudePersistentSession toggles keeping a long-lived claude-cli process per
// (session, agent) so warm turns ship only the new user message. On by default.
// See providers.CLISessionPool / _Docs/17.
func (t *Tunables) SetClaudePersistentSession(enabled bool) {
	t.mu.Lock()
	t.cliPersist = enabled
	t.mu.Unlock()
}

// ClaudePersistentSession reports whether the persistent claude-cli session path
// is enabled.
func (t *Tunables) ClaudePersistentSession() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cliPersist
}

// SetClaudeSysPromptFile toggles routing the appended claude-cli system prompt
// through a temp file (--append-system-prompt-file) instead of inline
// (--append-system-prompt). Off by default (inline). See _Docs/17.
func (t *Tunables) SetClaudeSysPromptFile(enabled bool) {
	t.mu.Lock()
	t.cliSysPromptFile = enabled
	t.mu.Unlock()
}

// ClaudeSysPromptFile reports whether the claude-cli system prompt is handed to
// the subprocess via a temp file (true) or inline (false, default).
func (t *Tunables) ClaudeSysPromptFile() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cliSysPromptFile
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

// SetCoordinatorLimits sets the coordinator/worker guards: the max number of
// active workers a single coordinator may run at once and the max auto-triggered
// coordinator turns per session (the notify-loop cap). A value of 0 selects the
// built-in default.
func (t *Tunables) SetCoordinatorLimits(maxWorkers, maxTurns int) {
	t.mu.Lock()
	t.coordMaxWorkers = maxWorkers
	t.coordMaxTurns = maxTurns
	t.mu.Unlock()
}

// CoordinatorMaxWorkers returns the cap on active workers per coordinator session
// (default when unset).
func (t *Tunables) CoordinatorMaxWorkers() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.coordMaxWorkers <= 0 {
		return DefaultCoordinatorMaxWorkers
	}
	return t.coordMaxWorkers
}

// CoordinatorMaxTurns returns the cap on auto-triggered coordinator turns per
// session (default when unset). 0 in settings means "use default"; the loop
// treats the returned value as a hard ceiling.
func (t *Tunables) CoordinatorMaxTurns() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.coordMaxTurns <= 0 {
		return DefaultCoordinatorMaxTurns
	}
	return t.coordMaxTurns
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

// SetProviderRetryMax configures the per-turn retry budget for transient
// provider faults (429/5xx/timeout). 0 disables retry; negative resets to the
// built-in default.
func (t *Tunables) SetProviderRetryMax(max int) {
	t.mu.Lock()
	t.providerRetryMax = max
	t.mu.Unlock()
}

// ProviderRetryMax returns the per-turn provider-retry budget. Negative (unset)
// resolves to the default; an explicit 0 disables retry.
func (t *Tunables) ProviderRetryMax() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.providerRetryMax < 0 {
		return DefaultProviderRetryMax
	}
	return t.providerRetryMax
}

// SetToolGuard configures the tool-loop guardrail: whether failing results get
// warning guidance appended, and whether the hard stop (block/halt thresholds)
// is armed.
func (t *Tunables) SetToolGuard(warnings, hardStop bool) {
	t.mu.Lock()
	t.toolGuardWarnings = warnings
	t.toolGuardHardStop = hardStop
	t.mu.Unlock()
}

// ToolGuardWarnings reports whether guardrail warning hints are enabled.
func (t *Tunables) ToolGuardWarnings() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.toolGuardWarnings
}

// ToolGuardHardStop reports whether the guardrail circuit breaker (block/halt)
// is armed.
func (t *Tunables) ToolGuardHardStop() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.toolGuardHardStop
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

// SetMaxOutputTokens configures the generation-cap override applied when a turn
// leaves MaxTokens unset. 0 = auto (resolve per model family); a positive value
// pins a fixed global cap across all models.
func (t *Tunables) SetMaxOutputTokens(n int) {
	t.mu.Lock()
	t.maxOutputTokens = n
	t.mu.Unlock()
}

// MaxOutputTokens returns the generation-cap override, or 0 when unset (auto:
// the caller resolves a per-model family default instead).
func (t *Tunables) MaxOutputTokens() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.maxOutputTokens < 0 {
		return 0
	}
	return t.maxOutputTokens
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

// defaultContextBudgetTokens mirrors conversation.defaultMaxTokens (the transcript
// budget) so tool-output thresholds can scale relative to it.
const defaultContextBudgetTokens = 12000

// budgetScaleLocked is the tool-threshold multiplier derived from the context
// budget (CG-9, second half): a larger MaxContextTokens means a single tool
// result may be larger before it's worth compacting. scale = budget / default,
// clamped to [1, 5] so thresholds never drop below the configured defaults and a
// huge budget can't explode them (5× ≈ 60KB System-B trigger, matching
// the external agent project's ~60KB summary ceiling). Both byte thresholds use the SAME factor,
// so the A-cap > B-threshold invariant holds at every scale. Caller holds the lock.
func (t *Tunables) budgetScaleLocked() float64 {
	return budgetScaleFor(t.contextBudgetTokens)
}

// budgetScaleFor is the pure scale calculation for an arbitrary budget (tokens),
// so callers can scale by a per-model budget instead of the stored process-wide
// one (see the compactor, which knows each turn's agent/model).
func budgetScaleFor(budgetTokens int) float64 {
	b := budgetTokens
	if b <= 0 {
		b = defaultContextBudgetTokens
	}
	scale := float64(b) / float64(defaultContextBudgetTokens)
	if scale < 1 {
		scale = 1
	}
	// Upper bound tracks conversation.budgetAutoCeil (128K) / default budget (12K)
	// ≈ 10.7, with a little headroom — so a model-aware budget at the ceiling is
	// not clipped, while a misconfigured huge budget still can't explode thresholds.
	if scale > 12 {
		scale = 12
	}
	return scale
}

// ContextBudgetTokens returns the configured transcript budget (0 = default), so
// the compactor can derive a per-model effective budget from it.
func (t *Tunables) ContextBudgetTokens() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.contextBudgetTokens
}

// CompactMaxBytes returns System A's hard byte cap, scaled to the stored
// process-wide budget (default when unset).
func (t *Tunables) CompactMaxBytes() int { return t.CompactMaxBytesFor(0) }

// CompactMaxBytesFor returns System A's hard byte cap scaled to the given budget
// (tokens); 0 → the stored process-wide budget. Lets the compactor scale by a
// per-model effective budget.
func (t *Tunables) CompactMaxBytesFor(budgetTokens int) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	base := t.compactMaxBytes
	if base <= 0 {
		base = DefaultCompactMaxBytes
	}
	if budgetTokens <= 0 {
		budgetTokens = t.contextBudgetTokens
	}
	return int(float64(base) * budgetScaleFor(budgetTokens))
}

// CompactLLM reports whether System B (LLM intent-aware summary) is on.
func (t *Tunables) CompactLLM() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.compactLLM
}

// CompactLLMThreshold returns the System B summary trigger, scaled to the stored
// process-wide budget (default when unset).
func (t *Tunables) CompactLLMThreshold() int { return t.CompactLLMThresholdFor(0) }

// CompactLLMThresholdFor returns the System B summary trigger scaled to the given
// budget (tokens); 0 → the stored process-wide budget. Lets the compactor scale
// by a per-model effective budget.
func (t *Tunables) CompactLLMThresholdFor(budgetTokens int) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	base := t.compactLLMThreshold
	if base <= 0 {
		base = DefaultCompactLLMThreshold
	}
	if budgetTokens <= 0 {
		budgetTokens = t.contextBudgetTokens
	}
	return int(float64(base) * budgetScaleFor(budgetTokens))
}

// SetContextBudget records the transcript token budget (settings.MaxContextTokens)
// so the tool-output byte thresholds scale with it (CG-9). 0 → default budget.
func (t *Tunables) SetContextBudget(maxContextTokens int) {
	t.mu.Lock()
	t.contextBudgetTokens = maxContextTokens
	t.mu.Unlock()
}

// CompactModel returns the dedicated model id for System B's summary, or "" when
// unset (callers then fall back to the title model, then the agent's own model).
func (t *Tunables) CompactModel() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.compactModel
}

// SetWorkdirGuards configures the working-directory safety guards: whether
// autonomous turns re-confine fs/shell to the working dir, whether autonomous
// sessions on a git repo get an isolated per-session worktree, and whether the
// boot/verification-sequence reminder is injected on autonomous turns.
func (t *Tunables) SetWorkdirGuards(autonomousConfine, gitWorktreeIsolation, autonomousBootSeq bool) {
	t.mu.Lock()
	t.autonomousConfine = autonomousConfine
	t.gitWorktreeIsolation = gitWorktreeIsolation
	t.autonomousBootSeq = autonomousBootSeq
	t.mu.Unlock()
}

// AutonomousConfine reports whether autonomous turns confine fs/shell to the
// working dir (the brake on the otherwise-unconfined tools).
func (t *Tunables) AutonomousConfine() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.autonomousConfine
}

// GitWorktreeIsolation reports whether autonomous sessions get a per-session git
// worktree instead of operating directly on the repository working tree.
func (t *Tunables) GitWorktreeIsolation() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.gitWorktreeIsolation
}

// AutonomousBootSeq reports whether autonomous turns get the boot/verification
// sequence reminder injected into their system prompt.
func (t *Tunables) AutonomousBootSeq() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.autonomousBootSeq
}

// SetHandoff configures the context-reset/handoff knobs: whether autonomous turns
// that hit the context limit auto-reset into a fresh session, the context-fill
// ratio that allows it, the max reset-chain depth, and whether the handoff is also
// written to a file in the working dir. Zero pressure/chain select the defaults.
func (t *Tunables) SetHandoff(auto bool, pressure float64, maxChain int, writeFile bool) {
	t.mu.Lock()
	t.handoffAuto = auto
	t.handoffPressure = pressure
	t.handoffMaxChain = maxChain
	t.handoffWriteFile = writeFile
	t.mu.Unlock()
}

// HandoffAuto reports whether automatic context-reset handoff is enabled.
func (t *Tunables) HandoffAuto() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.handoffAuto
}

// HandoffPressure returns the context-fill ratio above which an autonomous turn
// may auto-reset (default when unset).
func (t *Tunables) HandoffPressure() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.handoffPressure <= 0 {
		return DefaultHandoffPressure
	}
	return t.handoffPressure
}

// HandoffMaxChain returns the max reset-chain depth (default when unset).
func (t *Tunables) HandoffMaxChain() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.handoffMaxChain <= 0 {
		return DefaultHandoffMaxChain
	}
	return t.handoffMaxChain
}

// HandoffWriteFile reports whether the handoff is also written to a file in the
// session's working directory.
func (t *Tunables) HandoffWriteFile() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.handoffWriteFile
}

// SetProgress configures the persistent-progress knobs: whether the todo_write
// checklist is persisted to the project's progress file, and whether a fresh
// session injects a resumed-progress block from it.
func (t *Tunables) SetProgress(persist, resume bool) {
	t.mu.Lock()
	t.progressPersist = persist
	t.progressResume = resume
	t.mu.Unlock()
}

// ProgressPersist reports whether the checklist is persisted to disk.
func (t *Tunables) ProgressPersist() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.progressPersist
}

// ProgressResume reports whether a fresh session injects a resumed-progress block.
func (t *Tunables) ProgressResume() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.progressResume
}

// SetDebugJournal configures the per-session debug journal: whether structured
// observability events are emitted, and how many newest events are kept per
// session before the file is pruned (0 selects the built-in default).
func (t *Tunables) SetDebugJournal(enabled bool, cap int) {
	t.mu.Lock()
	t.debugJournal = enabled
	t.debugJournalCap = cap
	t.mu.Unlock()
}

// DebugJournalEnabled reports whether the parallel debug stream is emitted.
func (t *Tunables) DebugJournalEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.debugJournal
}

// DebugJournalCap returns how many newest debug events are kept per session
// (default when unset).
func (t *Tunables) DebugJournalCap() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.debugJournalCap <= 0 {
		return DefaultDebugJournalCap
	}
	return t.debugJournalCap
}

// SetAutoContinue configures autonomous self-completion: whether an autonomous
// turn that ends with unfinished work auto-issues continuation turns, and the max
// number of such extra turns per run. A value of 0 for max selects the built-in
// default (DefaultAutoContinueMax).
func (t *Tunables) SetAutoContinue(enabled bool, max int) {
	t.mu.Lock()
	t.autoContinue = enabled
	t.autoContinueMax = max
	t.mu.Unlock()
}

// AutoContinue reports whether autonomous auto-continue is enabled.
func (t *Tunables) AutoContinue() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.autoContinue
}

// AutoContinueMax returns the max number of auto-issued continuation turns per
// autonomous run (default when unset). A returned value is a hard ceiling.
func (t *Tunables) AutoContinueMax() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.autoContinueMax <= 0 {
		return DefaultAutoContinueMax
	}
	return t.autoContinueMax
}

// SetFileFreshnessGuard toggles the Edit/Write read-before-write freshness guard
// (Claude Code parity). On by default; disable to restore the historical behaviour
// where Edit/Write never check whether the file changed since it was last read.
func (t *Tunables) SetFileFreshnessGuard(enabled bool) {
	t.mu.Lock()
	t.fileFreshnessGuard = enabled
	t.mu.Unlock()
}

// FileFreshnessGuard reports whether the Edit/Write freshness guard is enabled.
func (t *Tunables) FileFreshnessGuard() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.fileFreshnessGuard
}

// SetAutoTagSessions toggles event-driven session auto-tagging (tool-error/error/
// goal/archived). On by default; disable to stop the runtime writing derived tags.
func (t *Tunables) SetAutoTagSessions(enabled bool) {
	t.mu.Lock()
	t.autoTagSessions = enabled
	t.mu.Unlock()
}

// AutoTagSessions reports whether event-driven auto-tagging is enabled.
func (t *Tunables) AutoTagSessions() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.autoTagSessions
}

// SetAutonomousTaskBudget sets the token budget announced to autonomous turns
// via the API-native task-budget directive. 0 disables; positive values below
// the API minimum (20K) are raised by the provider.
func (t *Tunables) SetAutonomousTaskBudget(tokens int) {
	t.mu.Lock()
	t.autonomousTaskBudget = tokens
	t.mu.Unlock()
}

// AutonomousTaskBudget returns the configured autonomous task budget (0 = off).
func (t *Tunables) AutonomousTaskBudget() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.autonomousTaskBudget < 0 {
		return 0
	}
	return t.autonomousTaskBudget
}

// SetNativeToolSearch toggles native (server-side) tool search on the anthropic
// tool loop: full catalog shipped with lazy tools deferred + the search server
// tool. Off by default (beta API surface).
func (t *Tunables) SetNativeToolSearch(enabled bool) {
	t.mu.Lock()
	t.nativeToolSearch = enabled
	t.mu.Unlock()
}

// SetProgrammaticTools toggles programmatic tool calling (code_execution +
// allowed_callers) on the native anthropic tool loop. Off by default.
func (t *Tunables) SetProgrammaticTools(enabled bool) {
	t.mu.Lock()
	t.programmaticTools = enabled
	t.mu.Unlock()
}

// ProgrammaticTools reports whether programmatic tool calling is enabled.
func (t *Tunables) ProgrammaticTools() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.programmaticTools
}

// SetWebTools toggles the server-side web search + web fetch tools on the
// native anthropic tool loop. Off by default.
func (t *Tunables) SetWebTools(enabled bool) {
	t.mu.Lock()
	t.webTools = enabled
	t.mu.Unlock()
}

// WebTools reports whether the server-side web tools are enabled.
func (t *Tunables) WebTools() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.webTools
}

// SetServerCompaction mirrors the API-native compaction beta into the agent
// layer (the tool loop needs it for the verbatim assistant echo).
func (t *Tunables) SetServerCompaction(enabled bool) {
	t.mu.Lock()
	t.serverCompaction = enabled
	t.mu.Unlock()
}

// ServerCompaction reports whether API-native compaction is enabled.
func (t *Tunables) ServerCompaction() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.serverCompaction
}

// NativeToolSearch reports whether native tool search is enabled.
func (t *Tunables) NativeToolSearch() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.nativeToolSearch
}

// SetMaxToolIters overrides the native agentic tool-loop iteration cap.
//   - max < 0   -> reset to the built-in default (see defaultMaxToolIters in toolloop.go)
//   - max == 0  -> UNLIMITED - the loop never breaks on its own (only recovery steps can)
//   - max > 0   -> cap to this many iterations per turn
//
// The value is read on every iteration of the tool loop, so a settings change
// takes effect on the next turn without a restart.
func (t *Tunables) SetMaxToolIters(max int) {
	t.mu.Lock()
	t.maxToolIters = max
	t.mu.Unlock()
}

// MaxToolIters returns the effective native tool-loop iteration cap, applying
// the default for negative / unset values. The caller is responsible for
// interpreting a returned value of 0 as "unlimited".
func (t *Tunables) MaxToolIters() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.maxToolIters < 0 {
		return defaultMaxToolIters
	}
	return t.maxToolIters
}
