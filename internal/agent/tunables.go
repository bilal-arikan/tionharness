package agent

import (
	"sync"
	"time"
)

// Default turn-recovery (A1) bounds, applied to a freshly constructed Tunables so
// test runtimes (which never call applySettings) get production-sane behaviour.
const (
	DefaultMaxTokenRetries    = 3 // resume attempts after the output-token cap
	DefaultReactiveKeepRecent = 6 // in-flight messages kept verbatim when compacting
	DefaultProviderRetryMax   = 2 // retries per turn for transient provider faults (429/5xx/timeout)
	DefaultStuckTurnThreshold = 3 // consecutive bad turns before a session is tagged "stuck" (autonomy refused)
)

// DefaultCoordinatorStallHaltTotal is the CUMULATIVE stall-nudge count at which a
// coordinator is hard-halted regardless of its in-memory streak. Same shape as
// DefaultStuckTurnThreshold, and deliberately the same number: three strikes of the
// same failure is where "the model is having a bad turn" becomes "the model is
// wedged". The two counters answer different questions and this tier reads the
// PERSISTENT one — see Session.StallNudges vs coordSlot.spawnHallucStreak:
//
//   - spawnHallucStreak is CONSECUTIVE and in-memory: a clean coordination call zeroes
//     it and a process restart erases it. It bounds the corrective nudges (max-nudges)
//     within one wedged stretch.
//   - Session.StallNudges is CUMULATIVE and persisted: it survives restarts, so a
//     coordinator that stalls, is nudged into one real call, then stalls again — over
//     and over, each stretch staying under the nudge cap — is still caught here.
//
// A genuinely clean coordinator turn resets the persistent counter too, so the
// threshold only ever fires on a coordinator that keeps relapsing without recovering.
// 0 disables this tier (the nudge-budget escalation still runs); <0 → this default.
const DefaultCoordinatorStallHaltTotal = 3

// Default spawn guards. They bound the fire-and-forget spawn_session surface so a
// burst of spawns can neither pin unbounded goroutines nor fan a single turn out
// into a spawn storm.
const (
	DefaultSpawnMaxConcurrent = 16 // max simultaneously-running spawned sessions
	DefaultSpawnMaxPerTurn    = 4  // max spawns one agent turn may launch
)

// DefaultSpawnTimeoutMinutes bounds a single background spawn work turn (plus the
// auto-continue continuations that share its context). Settings-driven
// (SpawnTimeoutMinutes) via applySettings; 0 selects this default.
const DefaultSpawnTimeoutMinutes = 20

// DefaultSpawnIdleTimeoutMinutes bounds INACTIVITY inside a background spawn/worker
// work turn: the idle watchdog cancels a turn that emits no step (tool/thinking/
// token) for this long, so a truly hung turn is reclaimed fast while a long-but-
// productive one (heavy exploration streaming tool calls) runs on up to the hard
// SpawnTimeout ceiling. Settings-driven (SpawnIdleTimeoutMin) via applySettings;
// 0 selects this default.
const DefaultSpawnIdleTimeoutMinutes = 5

// DefaultIdleResumeMax is the single-shot budget for the OUT-OF-LOOP idle-timeout
// resume (see runTurnWithIdleResume in turnoutcome.go): a background turn cut
// specifically by the idle watchdog (ErrTurnIdleTimeout) is auto-restarted this many
// times under a fresh idle window — continuing from its salvaged fragment — before it
// is reported unfinished. 1 = one resume (the SES17 / FND-708844f8 fix); 0 disables
// the resume entirely (turn is reported unfinished on the first idle cut, as it was
// before the fix). Settings-driven (IdleResumeMax) via applySettings.
const DefaultIdleResumeMax = 1

// DefaultTurnWatchdogMinutes bounds a single QUEUED turn (the serial per-session
// inbox worker in internal/api) before it is force-cancelled so the queue keeps
// moving. It is a wedge breaker, not a work budget: it must stay ABOVE every
// legitimate turn ceiling (spawn/schedule), else a healthy long turn — a heavy
// build/test loop streaming tool calls for an hour — is cut as if it were hung.
// TurnWatchdog() enforces that ordering. Settings-driven (TurnWatchdogMin) via
// applySettings; 0 selects this default.
const DefaultTurnWatchdogMinutes = 120

// DefaultTurnIdleWatchdogMinutes bounds INACTIVITY inside a queued turn: no event
// of any kind (tool step, thinking, token delta) reaching the session hub for this
// long means the turn is wedged, not busy. This is the measure that actually
// separates the two — wall clock cannot, which is why the hard ceiling above has to
// be generous and therefore leaves a real wedge running for hours. Sized well above
// the shell max timeout (a blocking command emits nothing until it returns).
// Settings-driven (TurnIdleWatchdogMin); 0 selects this default.
const DefaultTurnIdleWatchdogMinutes = 20

// DefaultScheduleTimeoutMinutes bounds a single scheduled fire (task run or prompt
// delivery / wake). Sized for current-generation models: one request can run many
// minutes and a multi-iteration tool loop longer still. Runaway protection comes
// from the loop guards (iteration cap, budgets), not this wall clock. Settings-driven
// (ScheduleTimeoutMinutes) via applySettings; 0 selects this default. Raised from
// 30 to 60: research-style scheduled prompts (search → fetch → synthesise over many
// sources) routinely spend that long in the tool loop and were being cut mid-run.
const DefaultScheduleTimeoutMinutes = 60

// Default coordinator/worker guards (see internal/agent/coordination.go). They
// bound the M2 coordination loop so a coordinator can neither fan out unbounded
// workers nor spin forever on worker → notify → new-turn feedback.
const (
	DefaultCoordinatorMaxWorkers = 8  // max active workers a single coordinator may run at once
	DefaultCoordinatorMaxTurns   = 50 // max auto-triggered coordinator turns per session (notify-loop cap)

	// UnlimitedCoordinatorTurns is the effective ceiling returned when the notify
	// loop is configured unlimited (settings value -1). It is a finite sentinel, not
	// a real infinity: the drain loop compares turns >= cap, so any value it can
	// never reach disables the cap while keeping the comparison total-order safe.
	UnlimitedCoordinatorTurns = 1 << 30

	// DefaultCoordinatorMaxDepth bounds how deep a coordinator TREE may nest: the
	// root coordinator is depth 0, its workers depth 1, and a worker may only be
	// spawned with coordinator mode on while its own children would still fit.
	//
	// A depth cap is not optional decoration. Worker count is per-coordinator, so
	// depth multiplies rather than adds: with the default 8 workers per node, depth
	// 5 already permits tens of thousands of sessions. 0 means unlimited, which is
	// supported but deliberately not the default — combine it with the subtree
	// budget below or a single runaway plan can spawn until the disk fills.
	DefaultCoordinatorMaxDepth = 5

	// DefaultCoordinatorSettleGraceSec — see agent.DefaultCoordinatorSettleGraceSec
	// in coordination_tree.go, which owns the rationale. Mirrored here so every
	// coordinator default reads from one block.

	// DefaultCoordinatorMaxSubtreeSessions bounds the TOTAL number of worker
	// sessions in one coordinator tree, across every level. The per-coordinator
	// worker cap cannot do this job: it is enforced per node, so N nodes each
	// legitimately under their own cap still add up without limit. This is the
	// budget that actually stops exponential fan-out. 0 means unlimited.
	DefaultCoordinatorMaxSubtreeSessions = 64
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
	titleProviderID  string
	shellEnabled     bool // gates the high-risk built-in `shell` tool (off by default)
	cliHooks         bool // pass PreToolUse/PostToolUse hooks to claude-cli via --settings (on by default)
	cliPersist       bool // keep a long-lived claude-cli process per session (on by default)
	cliSysPromptFile bool // hand claude-cli system prompt via temp file vs inline (off by default = inline)
	delegMaxDepth    int  // 0 → DefaultMaxDelegationDepth
	delegMaxCalls    int  // 0 → DefaultMaxDelegationCalls

	spawnMaxConcurrent  int // 0 → DefaultSpawnMaxConcurrent
	spawnMaxPerTurn     int // 0 → DefaultSpawnMaxPerTurn
	spawnTimeoutMin     int // 0 → DefaultSpawnTimeoutMinutes (spawn work-turn deadline, in minutes)
	spawnIdleTimeoutMin int // 0 → DefaultSpawnIdleTimeoutMinutes (spawn/worker inactivity watchdog, in minutes)
	idleResumeMax       int // <0 → DefaultIdleResumeMax; 0 = disabled; N = N single-shot idle-timeout resumes
	schedTimeoutMin     int // 0 → DefaultScheduleTimeoutMinutes (scheduled-fire deadline, in minutes)
	turnWatchdogMin     int // 0 → DefaultTurnWatchdogMinutes (queued-turn wedge breaker, in minutes)
	turnIdleWatchdogMin int // 0 → DefaultTurnIdleWatchdogMinutes (queued-turn inactivity window, in minutes)
	coordMaxWorkers     int // 0 → DefaultCoordinatorMaxWorkers
	coordMaxTurns       int // 0 → DefaultCoordinatorMaxTurns
	coordMaxDepth       int // 0 → DefaultCoordinatorMaxDepth (-1 = unlimited nesting)
	coordMaxSubtree     int // 0 → DefaultCoordinatorMaxSubtreeSessions (-1 = unlimited)
	coordSettleGrace    int // 0 → DefaultCoordinatorSettleGraceSec (upward-report backstop delay)

	// Coordinator stall/hallucination protection (see coordination_stall.go). The
	// master toggle gates the judge-based turn-end guard AND the staleness sweeper
	// that together catch a coordinator narrating a spawn it never issued (the SES1
	// freeze). coordStallSweepMin bounds the staleness window; <0 disables the
	// sweeper while leaving the turn-end guard on. coordStallMaxNudges caps the
	// consecutive corrective nudges before deferring to the sweeper / turn cap.
	coordStallGuard     bool // master switch (default on)
	coordStallSweepMin  int  // 0 → DefaultCoordinatorStallSweepMin; <0 disables the sweeper
	coordStallMaxNudges int  // 0 → DefaultCoordinatorStallMaxNudges
	// coordStallHaltTotal is the CUMULATIVE (persisted) stall count that hard-halts a
	// coordinator even when its in-memory streak keeps being reset. <0 →
	// DefaultCoordinatorStallHaltTotal; 0 disables this tier.
	coordStallHaltTotal int

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
	// Guardrail thresholds (<=0 → Default* constant). Warn thresholds apply to
	// the warning hints; block/halt fire only with the hard stop armed.
	guardExactWarn      int
	guardExactBlock     int
	guardSameToolWarn   int
	guardSameToolHalt   int
	guardNoProgressWarn int
	guardNoProgressBlck int

	// stuckTurnThreshold (Faz D): consecutive bad turns before a session is
	// tagged "stuck" and its autonomous turns are refused. 0 disables; <0 → default.
	stuckTurnThreshold int

	// lessonReflect (hata→ders döngüsü): a badly-ended turn spawns a background
	// reflection that distills the failure into a stored lesson, injected into
	// future turns' dynamic context. Costs one cheap-model call per failing turn.
	lessonReflect bool

	// language is the user's preferred reply language as a human name (e.g.
	// "Turkish (Türkçe)"), mirrored from Settings.Language so non-chat producers
	// (insight analyzer) can write their output in it. "" = model default.
	language string

	// Working-directory guards (fs/shell are otherwise unconfined).
	autonomousConfine bool // confine fs/shell to the working dir on autonomous turns (default on)

	// Autonomous boot/verification sequence (Anthropic long-running-agent harness
	// discipline). When on, headless turns get a short reminder to orient → recall
	// → select one task → verify the baseline → work → close the loop before acting.
	autonomousBootSeq bool // inject the boot-sequence reminder on autonomous turns (default on)

	// Context reset / handoff (Anthropic "harness design" pattern). When a long
	// autonomous turn runs up against the context limit, in-place compaction alone
	// leaves "context anxiety"; instead the runtime can write a handoff artifact and
	// spawn a FRESH session to continue in a clean window.
	// The trigger is the turn's overflow signal (reactive compaction fired), not a
	// fill-ratio threshold — see maybeAutoHandoff in handoff.go.
	handoffAuto      bool // auto-reset after an autonomous turn that hit the context limit (default off)
	handoffMaxChain  int  // max reset-chain depth before falling back to plain compaction (0 → DefaultHandoffMaxChain)
	handoffWriteFile bool // also write the handoff to <workdir>/.tionswarm/handoff.md (default off)

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
	// turn outcomes + session state (tool-error/error/archived) so an
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

// DefaultHandoffMaxChain caps consecutive context resets so a loop can't chain forever.
const DefaultHandoffMaxChain = 20

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
		idleResumeMax:      DefaultIdleResumeMax,
		// Guardrail warnings on by default (gentle nudge appended to failing
		// results); the hard stop stays opt-in from settings.
		toolGuardWarnings:  true,
		stuckTurnThreshold: DefaultStuckTurnThreshold,
		// Lesson reflection on by default: it only fires on failing turns (rare)
		// and uses the cheap title-model override when configured.
		lessonReflect: true,
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
		// Coordinator stall protection on by default (production overrides from
		// settings via SetCoordinatorStallGuard): it only acts on an idle coordinator
		// that made no coordination tool call, and the judge call is gated behind that
		// rare condition.
		coordStallGuard: true,
		// Cumulative-count halt tier on by default at the built-in threshold: it only
		// fires on a coordinator that has relapsed into phantom-spawning this many times
		// across its whole life, which no healthy coordinator ever reaches.
		coordStallHaltTotal: DefaultCoordinatorStallHaltTotal,
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

// SetTitleProviderID sets the provider used for auto-title generation. Empty
// means use the titling agent's own provider.
func (t *Tunables) SetTitleProviderID(id string) {
	t.mu.Lock()
	t.titleProviderID = id
	t.mu.Unlock()
}

// TitleProviderID returns the configured title-provider override ("" = agent default).
func (t *Tunables) TitleProviderID() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.titleProviderID
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

// SetSpawnTimeoutMinutes sets the deadline (in minutes) that bounds a single
// background spawn work turn and the auto-continue continuations sharing its
// context. A value of 0 selects the built-in default.
func (t *Tunables) SetSpawnTimeoutMinutes(minutes int) {
	t.mu.Lock()
	t.spawnTimeoutMin = minutes
	t.mu.Unlock()
}

// SpawnTimeout returns the spawn work-turn deadline as a duration (default when
// unset).
func (t *Tunables) SpawnTimeout() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := t.spawnTimeoutMin
	if m <= 0 {
		m = DefaultSpawnTimeoutMinutes
	}
	return time.Duration(m) * time.Minute
}

// SetSpawnIdleTimeoutMinutes sets the inactivity window (in minutes) after which
// the idle watchdog cancels a background spawn/worker turn that has emitted no
// step. 0 selects the built-in default.
func (t *Tunables) SetSpawnIdleTimeoutMinutes(minutes int) {
	t.mu.Lock()
	t.spawnIdleTimeoutMin = minutes
	t.mu.Unlock()
}

// SpawnIdleTimeout returns the spawn/worker inactivity window as a duration
// (default when unset).
func (t *Tunables) SpawnIdleTimeout() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := t.spawnIdleTimeoutMin
	if m <= 0 {
		m = DefaultSpawnIdleTimeoutMinutes
	}
	return time.Duration(m) * time.Minute
}

// SetIdleResumeMax configures how many times an idle-cut background turn is
// auto-restarted under a fresh idle window (see runTurnWithIdleResume). Negative
// selects the built-in default; 0 disables the resume.
func (t *Tunables) SetIdleResumeMax(max int) {
	t.mu.Lock()
	t.idleResumeMax = max
	t.mu.Unlock()
}

// IdleResumeMax returns the single-shot idle-resume budget. Negative (unset) maps to
// the default; 0 means the resume is disabled.
func (t *Tunables) IdleResumeMax() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.idleResumeMax < 0 {
		return DefaultIdleResumeMax
	}
	return t.idleResumeMax
}

// SetScheduleTimeoutMinutes sets the deadline (in minutes) that bounds a single
// scheduled fire (cron task/prompt delivery + schedule_wake). 0 selects the
// built-in default.
func (t *Tunables) SetScheduleTimeoutMinutes(minutes int) {
	t.mu.Lock()
	t.schedTimeoutMin = minutes
	t.mu.Unlock()
}

// ScheduleTimeout returns the scheduled-fire deadline as a duration (default when
// unset).
func (t *Tunables) ScheduleTimeout() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := t.schedTimeoutMin
	if m <= 0 {
		m = DefaultScheduleTimeoutMinutes
	}
	return time.Duration(m) * time.Minute
}

// SetTurnWatchdogMinutes sets the wall-clock ceiling (in minutes) after which a
// queued turn is force-cancelled so the per-session queue cannot stay wedged
// behind it. 0 selects the built-in default.
func (t *Tunables) SetTurnWatchdogMinutes(minutes int) {
	t.mu.Lock()
	t.turnWatchdogMin = minutes
	t.mu.Unlock()
}

// TurnWatchdog returns the queued-turn wedge breaker as a duration. It is floored
// at the LONGEST legitimate turn ceiling (spawn / schedule) so the safety net can
// never be tighter than the work it is meant to survive: a spawn turn allowed 120
// minutes must not be killed by a 20-minute queue watchdog. Configure it above
// those ceilings; the floor only rescues an inconsistent configuration.
func (t *Tunables) TurnWatchdog() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := t.turnWatchdogMin
	if m <= 0 {
		m = DefaultTurnWatchdogMinutes
	}
	for _, ceiling := range []int{t.spawnTimeoutMin, t.schedTimeoutMin} {
		if ceiling > m {
			m = ceiling
		}
	}
	return time.Duration(m) * time.Minute
}

// SetTurnIdleWatchdogMinutes sets the inactivity window (in minutes) after which a
// queued turn that has emitted nothing is force-cancelled. 0 selects the built-in
// default.
func (t *Tunables) SetTurnIdleWatchdogMinutes(minutes int) {
	t.mu.Lock()
	t.turnIdleWatchdogMin = minutes
	t.mu.Unlock()
}

// TurnIdleWatchdog returns the queued-turn inactivity window, capped at the hard
// ceiling: an idle window ABOVE the wall-clock cap could never fire, which would
// silently give back the very wedge detection it configures.
func (t *Tunables) TurnIdleWatchdog() time.Duration {
	hard := t.TurnWatchdog()
	t.mu.RLock()
	m := t.turnIdleWatchdogMin
	t.mu.RUnlock()
	if m <= 0 {
		m = DefaultTurnIdleWatchdogMinutes
	}
	if idle := time.Duration(m) * time.Minute; idle < hard {
		return idle
	}
	return hard
}

// SetCoordinatorLimits sets the coordinator/worker guards: the max number of
// active workers a single coordinator may run at once, the max auto-triggered
// coordinator turns per session (the notify-loop cap), how deep a coordinator
// TREE may nest, and how many worker sessions one whole tree may hold. A value of
// 0 selects the built-in default; -1 disables the depth/subtree bound entirely.
func (t *Tunables) SetCoordinatorLimits(maxWorkers, maxTurns, maxDepth, maxSubtree int) {
	t.mu.Lock()
	t.coordMaxWorkers = maxWorkers
	t.coordMaxTurns = maxTurns
	t.coordMaxDepth = maxDepth
	t.coordMaxSubtree = maxSubtree
	t.mu.Unlock()
}

// SetCoordinatorSettleGrace sets how long (seconds) the upward-report backstop
// waits after a sub-coordinator's branch goes quiet before auto-reporting for it.
// 0 selects the built-in default.
func (t *Tunables) SetCoordinatorSettleGrace(sec int) {
	t.mu.Lock()
	t.coordSettleGrace = sec
	t.mu.Unlock()
}

// CoordinatorSettleGrace returns the upward-report backstop delay. Tune it to the
// model behind the coordinators: too short and a slow synthesis turn loses the
// race, so the parent gets a needless "incomplete"; too long and a genuinely
// stalled branch keeps its coordinator waiting.
func (t *Tunables) CoordinatorSettleGrace() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	sec := t.coordSettleGrace
	if sec <= 0 {
		sec = DefaultCoordinatorSettleGraceSec
	}
	return time.Duration(sec) * time.Second
}

// SetCoordinatorStallGuard configures the coordinator stall/hallucination
// protection: the master toggle, the staleness sweeper window (minutes; 0 →
// default, <0 disables the sweeper), and the max consecutive nudges (0 → default).
func (t *Tunables) SetCoordinatorStallGuard(enabled bool, sweepMin, maxNudges int) {
	t.mu.Lock()
	t.coordStallGuard = enabled
	t.coordStallSweepMin = sweepMin
	t.coordStallMaxNudges = maxNudges
	t.mu.Unlock()
}

// CoordinatorStallGuard reports whether the judge-based stall protection is on.
func (t *Tunables) CoordinatorStallGuard() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.coordStallGuard
}

// CoordinatorStallSweep returns the staleness watchdog window as a duration, and
// whether the sweeper is enabled at all. A negative setting disables the sweeper
// (the turn-end guard still runs); 0 selects the built-in default.
func (t *Tunables) CoordinatorStallSweep() (window time.Duration, enabled bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.coordStallSweepMin < 0 {
		return 0, false
	}
	m := t.coordStallSweepMin
	if m == 0 {
		m = DefaultCoordinatorStallSweepMin
	}
	return time.Duration(m) * time.Minute, true
}

// CoordinatorStallMaxNudges returns the cap on consecutive corrective nudges
// (default when unset).
func (t *Tunables) CoordinatorStallMaxNudges() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.coordStallMaxNudges <= 0 {
		return DefaultCoordinatorStallMaxNudges
	}
	return t.coordStallMaxNudges
}

// SetCoordinatorStallHaltTotal configures the cumulative-count halt tier: the
// number of PERSISTED stall nudges (Session.StallNudges) at which a coordinator is
// hard-halted even though its in-memory streak never reached the nudge cap. 0
// disables the tier; negative selects the built-in default.
func (t *Tunables) SetCoordinatorStallHaltTotal(n int) {
	t.mu.Lock()
	t.coordStallHaltTotal = n
	t.mu.Unlock()
}

// CoordinatorStallHaltTotal returns the cumulative stall threshold that hard-halts a
// coordinator. A returned 0 means the tier is disabled, which is a valid setting.
func (t *Tunables) CoordinatorStallHaltTotal() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.coordStallHaltTotal < 0 {
		return DefaultCoordinatorStallHaltTotal
	}
	return t.coordStallHaltTotal
}

// CoordinatorMaxDepth returns how deep a coordinator tree may nest (root = depth
// 0), or 0 for unlimited nesting. Settings pass -1 to mean unlimited; 0 there
// means "use the default", so the two are mapped apart here — a caller only ever
// has to check for 0.
func (t *Tunables) CoordinatorMaxDepth() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	switch {
	case t.coordMaxDepth < 0:
		return 0 // explicitly unlimited
	case t.coordMaxDepth == 0:
		return DefaultCoordinatorMaxDepth
	default:
		return t.coordMaxDepth
	}
}

// CoordinatorMaxSubtreeSessions returns the cap on the total number of worker
// sessions in ONE coordinator tree (all levels combined), or 0 for unlimited.
// Same 0-vs--1 mapping as CoordinatorMaxDepth.
func (t *Tunables) CoordinatorMaxSubtreeSessions() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	switch {
	case t.coordMaxSubtree < 0:
		return 0 // explicitly unlimited
	case t.coordMaxSubtree == 0:
		return DefaultCoordinatorMaxSubtreeSessions
	default:
		return t.coordMaxSubtree
	}
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
// session. 0 in settings means "use default"; -1 means unlimited (returns the
// UnlimitedCoordinatorTurns sentinel, which the drain loop can never reach); any
// positive value is a hard ceiling. Same 0-vs--1 mapping as the subtree budget.
func (t *Tunables) CoordinatorMaxTurns() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	switch {
	case t.coordMaxTurns < 0:
		return UnlimitedCoordinatorTurns
	case t.coordMaxTurns == 0:
		return DefaultCoordinatorMaxTurns
	default:
		return t.coordMaxTurns
	}
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

// SetToolGuardThresholds configures the six guardrail thresholds. Values <= 0
// select the built-in defaults (2/5, 3/8, 2/5).
func (t *Tunables) SetToolGuardThresholds(exactWarn, exactBlock, sameToolWarn, sameToolHalt, noProgressWarn, noProgressBlock int) {
	t.mu.Lock()
	t.guardExactWarn = exactWarn
	t.guardExactBlock = exactBlock
	t.guardSameToolWarn = sameToolWarn
	t.guardSameToolHalt = sameToolHalt
	t.guardNoProgressWarn = noProgressWarn
	t.guardNoProgressBlck = noProgressBlock
	t.mu.Unlock()
}

// ToolGuardThresholds returns the six guardrail thresholds as configured
// (values <= 0 mean "use the default"; newToolGuard resolves them).
func (t *Tunables) ToolGuardThresholds() (exactWarn, exactBlock, sameToolWarn, sameToolHalt, noProgressWarn, noProgressBlock int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.guardExactWarn, t.guardExactBlock, t.guardSameToolWarn, t.guardSameToolHalt, t.guardNoProgressWarn, t.guardNoProgressBlck
}

// SetStuckTurnThreshold configures the consecutive bad-turn count that tags a
// session "stuck" and suspends its autonomous turns. 0 disables the gate;
// negative resets to the built-in default.
func (t *Tunables) SetStuckTurnThreshold(n int) {
	t.mu.Lock()
	t.stuckTurnThreshold = n
	t.mu.Unlock()
}

// StuckTurnThreshold returns the stuck-session threshold (0 = gate disabled).
func (t *Tunables) StuckTurnThreshold() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.stuckTurnThreshold < 0 {
		return DefaultStuckTurnThreshold
	}
	return t.stuckTurnThreshold
}

// SetLessonReflect toggles the failure→lesson reflection loop.
func (t *Tunables) SetLessonReflect(enabled bool) {
	t.mu.Lock()
	t.lessonReflect = enabled
	t.mu.Unlock()
}

// LessonReflect reports whether the failure→lesson reflection loop is on.
func (t *Tunables) LessonReflect() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lessonReflect
}

// SetLanguage stores the user's preferred reply language as a human name (e.g.
// "Turkish (Türkçe)"), so non-chat producers (insight analyzer, ...) can write
// their output in it. Empty = no preference (model default).
func (t *Tunables) SetLanguage(name string) {
	t.mu.Lock()
	t.language = name
	t.mu.Unlock()
}

// Language returns the user's preferred reply language name ("" = unset).
func (t *Tunables) Language() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.language
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

// SetWorkdirGuards configures the working-directory safety guards: whether
// autonomous turns re-confine fs/shell to the working dir, and whether the
// boot/verification-sequence reminder is injected on autonomous turns.
func (t *Tunables) SetWorkdirGuards(autonomousConfine, autonomousBootSeq bool) {
	t.mu.Lock()
	t.autonomousConfine = autonomousConfine
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

// AutonomousBootSeq reports whether autonomous turns get the boot/verification
// sequence reminder injected into their system prompt.
func (t *Tunables) AutonomousBootSeq() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.autonomousBootSeq
}

// SetHandoff configures the context-reset/handoff knobs: whether autonomous turns
// that hit the context limit auto-reset into a fresh session, the max reset-chain
// depth, and whether the handoff is also written to a file in the working dir. A
// zero chain selects the default.
func (t *Tunables) SetHandoff(auto bool, maxChain int, writeFile bool) {
	t.mu.Lock()
	t.handoffAuto = auto
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
// archived). On by default; disable to stop the runtime writing derived tags.
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
