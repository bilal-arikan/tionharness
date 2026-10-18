package agent

import (
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/decider"
)

// Default turn-recovery (A1) bounds, applied to a freshly constructed Tunables so
// test runtimes (which never call applySettings) get production-sane behaviour.
const (
	DefaultMaxTokenRetries    = 3 // resume attempts after the output-token cap
	DefaultReactiveKeepRecent = 6 // in-flight messages kept verbatim when compacting
	DefaultProviderRetryMax   = 2 // retries per turn for transient provider faults (429/5xx/timeout)
	DefaultStuckTurnThreshold = 3 // consecutive bad turns before a session is tagged "stuck" (autonomy refused)
)

// Default spawn guards. They bound the fire-and-forget spawn_session surface so a
// burst of spawns can neither pin unbounded goroutines nor fan a single turn out
// into a spawn storm.
const (
	DefaultSpawnMaxConcurrent = 4 // max simultaneously-running spawned sessions
	DefaultSpawnQueueMax      = 8 // max waiting spawned sessions
	DefaultSpawnMaxPerTurn    = 4 // max spawns one agent turn may launch
)

// DefaultSpawnIdleTimeoutMinutes bounds INACTIVITY inside a background spawn
// work turn: the idle watchdog cancels a turn that emits no step (tool/thinking/
// token) for this long, so a truly hung turn is reclaimed fast while a long-but-
// productive one (heavy exploration streaming tool calls) runs on. Settings-driven (SpawnIdleTimeoutMin) via applySettings;
// 0 selects this default.
const DefaultSpawnIdleTimeoutMinutes = 10

// Interactive chat turn bounds. The legacy hard ceiling defaults disabled; the
// semantic idle window reclaims a
// turn whose PROVIDER STREAM died silently — the socket stays half-open, the CLI
// subprocess never exits, and without this the session hangs forever and its
// queued inbox messages are never delivered. 3 minutes (180s) of complete
// silence is far longer than any real inter-step gap on a streaming provider,
// and a long-but-step-less operation (non-streaming completion, one big tool
// call) is bounded by a separate operation lease derived from the same window.
const (
	DefaultChatTurnIdleTimeoutMinutes = 3
)

// DefaultIdleResumeMax is the single-shot budget for the OUT-OF-LOOP idle-timeout
// resume (see runTurnWithIdleResume in turnoutcome.go): a background turn cut
// specifically by the idle watchdog (ErrTurnIdleTimeout) is auto-restarted this many
// times under a fresh idle window — continuing from its salvaged fragment — before it
// is reported unfinished. 1 = one resume (the SES17 / FND-708844f8 fix); 0 disables
// the resume entirely (turn is reported unfinished on the first idle cut, as it was
// before the fix). Settings-driven (IdleResumeMax) via applySettings.
const DefaultIdleResumeMax = 1

// DefaultTurnIdleWatchdogMinutes bounds semantic inactivity inside a queued run.
// Only new assistant/tool output and terminal state transitions advance it.
// Settings-driven (TurnIdleWatchdogMin); 0 selects this default.
const DefaultTurnIdleWatchdogMinutes = 20

// Tunables holds process-wide, settings-driven knobs that cut across every
// workspace runtime: an optional model override for auto-title generation and
// context/journal/memory controls. (Autonomy pausing is per-workspace — see
// Runtime.SetPaused — so it is not a process-wide knob.) A single instance is created at boot and
// shared (by pointer) with every Runtime and the API server, so a settings
// change applies uniformly regardless of which workspace runtime reads it.
type Tunables struct {
	mu               sync.RWMutex
	shellEnabled     bool // gates the high-risk built-in `shell` tool (off by default)
	cliHooks         bool // pass PreToolUse/PostToolUse hooks to claude-cli via --settings (on by default)
	cliPersist       bool // keep a long-lived claude-cli process per session (on by default)
	cliSysPromptFile bool // hand claude-cli system prompt via temp file vs inline (off by default = inline)
	// cliToolAllowlist pins claude-cli's built-in tool menu to the natives a turn
	// actually needs (`--tools`), instead of only suppressing the shadowed ones
	// (`--disallowedTools`). The CLI prompt scales with the tools it carries — the
	// full menu is ~36k tokens per session, the worker menu ~11k (_Docs/17). On by
	// default.
	cliToolAllowlist bool
	// cliNativeSubagents keeps claude-cli's own Agent launcher on the menu for the
	// read-only research types (Explore/Plan); their transcript is folded into the
	// trace. On by default.
	cliNativeSubagents bool
	delegMaxDepth      int // 0 → DefaultMaxDelegationDepth
	delegMaxCalls      int // 0 → DefaultMaxDelegationCalls

	spawnMaxConcurrent  int // 0 → DefaultSpawnMaxConcurrent
	spawnQueueMax       int // 0 → DefaultSpawnQueueMax
	flowRunRetention    int // finished root runs kept per flow; 0 → unlimited
	spawnMaxPerTurn     int // 0 → DefaultSpawnMaxPerTurn
	spawnIdleTimeoutMin int // 0 → DefaultSpawnIdleTimeoutMinutes (spawn inactivity watchdog, in minutes)
	chatTurnIdleMin     int // 0 = disabled (interactive chat turn inactivity window, in minutes)
	idleResumeMax       int // <0 → DefaultIdleResumeMax; 0 = disabled; N = N single-shot idle-timeout resumes
	turnIdleWatchdogMin int // 0 → DefaultTurnIdleWatchdogMinutes (queued-turn inactivity window, in minutes)
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
	handoffWriteFile bool // also write the handoff to <workdir>/.tionharness/handoff.md (default off)

	// Persistent progress (Anthropic claude-progress convention). When on, the
	// todo_write checklist is persisted to <cwd>/.tionharness/progress.json so it
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

	// autoTagSessions — when true, the runtime derives well-known session tags from
	// turn outcomes + session state (tool-error/error/archived) so an
	// automation can scan + repair them. Default on. See internal/agent/autotag.go.
	autoTagSessions bool

	// codeMode (_Docs/44) — when true (and the shell gate is on), the MCP
	// catalog is additionally exposed as generated Python bindings behind the
	// run_code tool (code execution with MCP: schemas stay out of context,
	// intermediate data stays in the execution environment). Settings-driven
	// (enableCodeMode, default off); TIONHARNESS_CODE_MODE=1 seeds the setting at
	// boot. See codemode_tunable.go.
	codeMode bool

	// decider is the app-wide decision-model hub (internal/decider), shared by
	// every workspace runtime. nil until the API server wires it (tests: never),
	// in which case every decision authority behaves as off. See tunables_decider.go.
	decider *decider.Hub
}

// DefaultDebugJournalCap mirrors db.DefaultDebugJournalCap as the resolved
// default when no explicit cap is configured.
const DefaultDebugJournalCap = 5000

// DefaultAutoContinueMax bounds how many extra turns an autonomous run will
// auto-issue to finish work the agent left pending, when no explicit max is set.
const DefaultAutoContinueMax = 3

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
		chatTurnIdleMin:    DefaultChatTurnIdleTimeoutMinutes,
		// Guardrail warnings on by default (gentle nudge appended to failing
		// results); the hard stop stays opt-in from settings.
		toolGuardWarnings:  true,
		stuckTurnThreshold: DefaultStuckTurnThreshold,
		// Lesson reflection on by default: it only fires on failing turns (rare)
		// and uses the cheap title-model override when configured.
		lessonReflect: true,
		// claude-cli built-in tool allowlist + auxiliary native routing on by
		// default: both are pure prefix-size levers (_Docs/17, 2026-09-03).
		cliToolAllowlist:   true,
		cliNativeSubagents: true,
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
		// Auto-tagging on by default (production overrides from settings via
		// SetAutoTagSessions); test runtimes that skip applySettings still auto-tag.
		autoTagSessions: true,
	}
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
// default; turn off to keep hooks native-only when a hook authored for TionHarness's
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

// SetClaudeCLIToolAllowlist toggles pinning claude-cli's built-in tool menu to
// the per-turn native allowlist (`--tools`). See climcp.NativeToolAllowlist.
func (t *Tunables) SetClaudeCLIToolAllowlist(enabled bool) {
	t.mu.Lock()
	t.cliToolAllowlist = enabled
	t.mu.Unlock()
}

// ClaudeCLIToolAllowlist reports whether claude-cli turns carry a `--tools`
// allowlist of built-ins.
func (t *Tunables) ClaudeCLIToolAllowlist() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cliToolAllowlist
}

// SetClaudeCLINativeSubagents toggles claude-cli's native Agent launcher for the
// read-only research types (Explore/Plan). See climcp.WriteConfig.
func (t *Tunables) SetClaudeCLINativeSubagents(enabled bool) {
	t.mu.Lock()
	t.cliNativeSubagents = enabled
	t.mu.Unlock()
}

// ClaudeCLINativeSubagents reports whether claude-cli's native research
// subagents (Explore/Plan) stay on the menu.
func (t *Tunables) ClaudeCLINativeSubagents() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cliNativeSubagents
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
func (t *Tunables) SetSpawnLimits(maxConcurrent, queueMax, maxPerTurn int) {
	t.mu.Lock()
	t.spawnMaxConcurrent = maxConcurrent
	t.spawnQueueMax = queueMax
	t.spawnMaxPerTurn = maxPerTurn
	t.mu.Unlock()
}

// SetFlowRunRetention sets how many finished ROOT runs to keep per flow; older
// trees are pruned by the flow sweeper. <= 0 keeps everything (_Docs/77 R8).
func (t *Tunables) SetFlowRunRetention(keep int) {
	t.mu.Lock()
	t.flowRunRetention = keep
	t.mu.Unlock()
}

// FlowRunRetention returns the per-flow finished-run retention (0 = unlimited).
func (t *Tunables) FlowRunRetention() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.flowRunRetention < 0 {
		return 0
	}
	return t.flowRunRetention
}

// SpawnQueueMax returns the cap on queued spawned sessions (default when unset).
func (t *Tunables) SpawnQueueMax() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.spawnQueueMax <= 0 {
		return DefaultSpawnQueueMax
	}
	return t.spawnQueueMax
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

// SetSpawnIdleTimeoutMinutes sets the inactivity window (in minutes) after which
// the idle watchdog cancels a background spawn turn that has emitted no
// step. 0 selects the built-in default.
func (t *Tunables) SetSpawnIdleTimeoutMinutes(minutes int) {
	t.mu.Lock()
	t.spawnIdleTimeoutMin = minutes
	t.mu.Unlock()
}

// SpawnIdleTimeout returns the spawn inactivity window as a duration
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

func (t *Tunables) SetChatTurnIdleTimeoutMinutes(minutes int) {
	t.mu.Lock()
	t.chatTurnIdleMin = minutes
	t.mu.Unlock()
}

func (t *Tunables) ChatTurnIdleTimeout() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return time.Duration(t.chatTurnIdleMin) * time.Minute
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

// SetTurnIdleWatchdogMinutes sets the inactivity window (in minutes) after which a
// queued turn that has emitted nothing is force-cancelled. 0 selects the built-in
// default.
func (t *Tunables) SetTurnIdleWatchdogMinutes(minutes int) {
	t.mu.Lock()
	t.turnIdleWatchdogMin = minutes
	t.mu.Unlock()
}

// TurnIdleWatchdog returns the semantic inactivity window.
func (t *Tunables) TurnIdleWatchdog() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := t.turnIdleWatchdogMin
	if m <= 0 {
		m = DefaultTurnIdleWatchdogMinutes
	}
	return time.Duration(m) * time.Minute
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
