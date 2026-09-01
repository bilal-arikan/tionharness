// Package settings holds the application-global, user-editable configuration
// that backs the Settings screen. It is a single JSON document persisted in the
// data directory; the sensitive Anthropic API key is stored AES-GCM encrypted
// and never returned to clients in plaintext.
package settings

import (
	"os"
	"path/filepath"
)

// defaultClaudeConfigDir is the baseline CLAUDE_CONFIG_DIR for claude-cli: a
// TionHarness-managed, isolated config home (~/.tionharness/claude-home) so the CLI runs
// against a clean skills/settings/commands/login set instead of the user's shared
// ~/.claude out of the box. Requires a one-time `claude` login in that directory.
// If the home dir can't be resolved we return "" — that is the correct fallback
// (inherit the ambient ~/.claude), not a swallowed error.
func defaultClaudeConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".tionharness", "claude-home")
}

// defaultCodexConfigDir is the baseline CODEX_HOME for codex-cli, the sibling of
// defaultClaudeConfigDir: a TionHarness-managed config home (~/.tionharness/codex-home)
// so the CLI runs against a config.toml and login we own rather than the user's
// shared ~/.codex. Requires a one-time `codex login` in that directory. As with
// the claude side, an unresolvable home dir yields "" — inheriting the ambient
// ~/.codex is the correct fallback, not a swallowed error.
func defaultCodexConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".tionharness", "codex-home")
}

// Theme options for the UI.
const (
	ThemeDark   = "dark"
	ThemeLight  = "light"
	ThemeSystem = "system"
)

// AutoCompactMode options — see Settings.AutoCompactMode.
const (
	AutoCompactRolling = "rolling"
	AutoCompactNative  = "native"
	AutoCompactAuto    = "auto"
)

// CustomProvider is a user-added OpenAI- or Anthropic-compatible endpoint. Its
// ID is used as a provider identifier (Agent.Provider) and must not collide
// with a built-in. KeyEnc is AES-GCM and never serialized to the API.
type CustomProvider struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`         // "openai" | "anthropic"
	BaseURL      string `json:"baseUrl"`      // API base (no trailing path)
	DefaultModel string `json:"defaultModel"` // applied when a request omits one
	Models       string `json:"models"`       // optional model-id suggestions (comma/newline)
	KeyEnc       string `json:"keyEnc"`       // AES-GCM, never exposed
	// Reasoning: endpoint accepts a reasoning-effort control (see market.ProviderPayload).
	Reasoning bool `json:"reasoning,omitempty"`
	// PromptCache: "native" | "auto" | "none" | "" (unknown) — see market.ProviderPayload.
	PromptCache string `json:"promptCache,omitempty"`
}

// Settings is the full, persisted configuration document. The encrypted
// Anthropic key lives in AnthropicKeyEnc and is never serialized to the API
// (json tag "-"); clients see only AnthropicKeySet via the DTO.
type Settings struct {
	// Appearance.
	Theme       string `json:"theme"`
	Accent      string `json:"accent"`      // hex color, e.g. "#4f8cff"
	ThemePreset string `json:"themePreset"` // theme color + variant id, e.g. "violet-dark" ("" = default)
	// Language is the AGENT reply language (system-prompt level). UILanguage is
	// the interface language; "" means it follows Language. See language.go for
	// why the two axes are deliberately separate.
	Language   string `json:"language"`   // see SupportedLanguages
	UILanguage string `json:"uiLanguage"` // see SupportedLanguages; "" = follow Language

	// Providers. There is no abstract app-global "default provider/model": a new
	// agent inherits the first existing agent's concrete provider/model, falling
	// back to the built-in claude-cli (local, keyless) + the provider's own default
	// model only when no agent exists yet.
	// DefaultPermissionMode seeds new agents' tool-use permission gate:
	// "read-only" | "ask" | "auto". "" falls back to "auto".
	DefaultPermissionMode string `json:"defaultPermissionMode"`
	// ClaudeCLIPath and CodexCLIPath: no live functional reader remains outside
	// MigrateFromSettings (kept for that one-time boot migration; never exposed
	// to the API — see DTO/Patch below).
	ClaudeCLIPath   string `json:"claudeCliPath"`   // "" = auto-detect on PATH
	ClaudeConfigDir string `json:"claudeConfigDir"` // CLAUDE_CONFIG_DIR for claude-cli; "" = inherit ~/.claude
	// claude-cli credential injected into the subprocess env so an isolated config
	// dir authenticates without an interactive in-dir `claude login`. The token is
	// AES-GCM encrypted (never serialized to the API); the kind selects the env var:
	//   "oauth"  → CLAUDE_CODE_OAUTH_TOKEN (Max/Pro subscription, from `claude setup-token`)
	//   "apikey" → ANTHROPIC_API_KEY       (API billing)
	//   ""       → none injected (rely on the config dir's own login)
	ClaudeCliAuthKind     string `json:"claudeCliAuthKind"`
	ClaudeCliAuthTokenEnc string `json:"claudeCliAuthTokenEnc"` // AES-GCM, never exposed
	// codex-cli (the second keyless CLI transport). CodexCLIPath resolves the
	// binary; CodexConfigDir is its CODEX_HOME — the exact analogue of
	// CLAUDE_CONFIG_DIR, holding the subscription login (auth.json) and the
	// generated config.toml. There is no token-injection counterpart: Codex has
	// no env-var credential channel, so authentication is always the config
	// home's own `codex login`.
	CodexCLIPath   string `json:"codexCliPath"`   // "" = auto-detect on PATH
	CodexConfigDir string `json:"codexConfigDir"` // CODEX_HOME for codex-cli; "" = inherit ~/.codex

	AnthropicKeyEnc string `json:"anthropicKeyEnc"` // AES-GCM, never exposed

	// MiniMax (OpenAI-compatible) provider.
	MinimaxKeyEnc  string `json:"minimaxKeyEnc"` // AES-GCM, never exposed
	MinimaxBaseURL string `json:"minimaxBaseUrl"`

	// OpenRouter (OpenAI-compatible) provider — one key, hundreds of models.
	OpenRouterKeyEnc  string `json:"openrouterKeyEnc"` // AES-GCM, never exposed
	OpenRouterBaseURL string `json:"openrouterBaseUrl"`

	// Z.ai GLM (Anthropic-compatible) provider — GLM family, one key.
	ZAIKeyEnc  string `json:"zaiKeyEnc"` // AES-GCM, never exposed
	ZAIBaseURL string `json:"zaiBaseUrl"`

	// DeepSeek (OpenAI-compatible) provider — DeepSeek V4 family, one key.
	DeepSeekKeyEnc  string `json:"deepseekKeyEnc"` // AES-GCM, never exposed
	DeepSeekBaseURL string `json:"deepseekBaseUrl"`

	// CustomProviders are user-added OpenAI- or Anthropic-compatible endpoints
	// (OpenRouter, Gemini, Kimi, Ollama, ...). Each is selectable as a provider
	// id alongside the built-ins; the key is AES-GCM encrypted like the others.
	CustomProviders []CustomProvider `json:"customProviders"`

	// Anthropic beta capabilities (anthropic provider only; claude-cli ignores).
	// (The 1M-context beta was retired — 1M is GA since 2026-03, no toggle needed.)
	ExtendedPromptCache bool `json:"extendedPromptCache"` // 1h extended prompt cache TTL
	// AnthropicContextEditing enables API-native context editing (server-side
	// clear_tool_uses on the cached prefix — the microcompact analogue, P3). Off by
	// default; complements the client-side compaction, does not replace it.
	AnthropicContextEditing bool `json:"anthropicContextEditing"`
	// AnthropicNativeToolSearch (beta) ships the FULL tool catalog on native
	// anthropic tool turns with lazy tools marked defer_loading plus the
	// server-side tool-search tool: discovery without an activate_tools
	// round-trip, cache-safe schema appends, byte-stable tools block. Off by
	// default. First-party anthropic provider only.
	AnthropicNativeToolSearch bool `json:"anthropicNativeToolSearch"`
	// AutonomousTaskBudgetTokens (beta), when > 0, announces a token budget to
	// every AUTONOMOUS turn via the API-native task-budget directive
	// (output_config.task_budget): the model sees a running countdown for the
	// whole agentic loop and paces itself. Values below the API minimum (20000)
	// are raised to it. 0 = off. Adaptive-class anthropic models only.
	AutonomousTaskBudgetTokens int `json:"autonomousTaskBudgetTokens"`
	// AnthropicProgrammaticTools enables programmatic tool calling on native
	// anthropic tool turns: the code-execution server tool is added and eligible
	// builtins become callable from Claude-written Python in Anthropic's
	// container — intermediate results never enter context. MCP + interactive
	// tools excluded. Off by default. First-party anthropic provider only.
	AnthropicProgrammaticTools bool `json:"anthropicProgrammaticTools"`
	// AnthropicWebTools adds the SERVER-SIDE web search + web fetch tools to
	// native anthropic tool turns: searches run on Anthropic's infrastructure
	// (no local process) and return cited results in the same response. Billed
	// per search — conservative per-turn max_uses ceilings are applied. Off by
	// default. First-party anthropic provider only.
	AnthropicWebTools bool `json:"anthropicWebTools"`
	// AnthropicServerCompaction enables API-native compaction (beta): the server
	// summarizes earlier history into compaction blocks once the prompt nears
	// its trigger (~150K tokens); within a turn the blocks are echoed back
	// verbatim. Complements the client-side compaction (which keeps managing
	// the cross-turn transcript). Off by default. anthropic provider only.
	AnthropicServerCompaction bool `json:"anthropicServerCompaction"`
	// AnthropicRefusalFallback (beta) attaches the server-side fallback to
	// Fable-class requests: a safety-classifier decline (stop_reason "refusal")
	// is transparently re-served by Opus 4.8 inside the same call instead of
	// failing the turn (a decline before output isn't billed; the rescue bills
	// at Opus rates). ON by default per Anthropic's Fable guidance — false
	// positives on benign adjacent work do happen. anthropic provider only.
	AnthropicRefusalFallback bool `json:"anthropicRefusalFallback"`

	// Desktop / display behaviour (applied client-side).
	DesktopNotifications bool `json:"desktopNotifications"` // browser notifications
	KeepAwake            bool `json:"keepAwake"`            // hold a screen wake lock

	// User profile — injected as context so agents address the user correctly.
	UserName     string `json:"userName"`
	UserTimezone string `json:"userTimezone"`
	UserCity     string `json:"userCity"`
	UserCountry  string `json:"userCountry"`
	UserNotes    string `json:"userNotes"`

	// Context.
	MaxContextTokens int `json:"maxContextTokens"`
	KeepRecentMsgs   int `json:"keepRecentMsgs"`
	// Model-aware transcript budget (see internal/conversation/budget.go). The live
	// budget is clamp(window * ContextBudgetFraction, MaxContextTokens, ContextBudgetCeil)
	// when the model's context window is known. ContextBudgetFraction = 0 means "auto"
	// → a context-rot-aware per-family share (providers.AdaptiveBudgetFraction); a
	// positive value pins a fixed manual share. ContextBudgetCeil is the operative cap
	// for big-window (1M) models — raise it to keep more history verbatim before the
	// first silent compaction (trades recall precision for raw history, _Docs/17 §12).
	ContextBudgetCeil     int     `json:"contextBudgetCeil"`
	ContextBudgetFraction float64 `json:"contextBudgetFraction"`

	// AutoCompactMode picks WHAT happens when automatic context compaction fires:
	// "rolling" = TionHarness' own rolling-summary fold (today's behaviour),
	// "native"  = the CLI provider's own native compaction,
	// "auto"    = native when the provider supports it and a warm CLI session is
	//             live, rolling otherwise.
	AutoCompactMode string `json:"autoCompactMode"`

	// Context reset / handoff (Anthropic "harness design"). When HandoffAuto is on,
	// an autonomous turn that hits the context limit writes a handoff artifact and
	// continues in a FRESH session instead of only compacting in place.
	// The trigger is the turn's overflow signal, not a fill-ratio threshold.
	// HandoffMaxChain caps consecutive resets; HandoffWriteFile also drops the handoff
	// to a file in the working dir. 0 values select the built-in defaults.
	HandoffAuto      bool `json:"handoffAuto"`
	HandoffMaxChain  int  `json:"handoffMaxChain"`
	HandoffWriteFile bool `json:"handoffWriteFile"`

	// Persistent progress (Anthropic claude-progress convention). When
	// ProgressPersist is on, the todo_write checklist is persisted to
	// <cwd>/.tionharness/progress.json so it survives across sessions; ProgressResume
	// injects it back into a fresh session's context at start.
	ProgressPersist bool `json:"progressPersist"`
	ProgressResume  bool `json:"progressResume"`

	// AutonomousAutoContinue (autonomous self-completion). When on, an autonomous
	// turn (scheduler/spawn/wake) that ends with UNFINISHED work — it left open
	// todo items, or its last action was a lazy tool activation whose tools only
	// take effect on the next turn — is automatically followed by a continuation
	// turn, up to AutonomousAutoContinueMax times, so unattended work self-completes
	// instead of stalling. Each continuation is budget-gated; the loop also stops as
	// soon as a turn makes no tool progress. 0 max selects the built-in default (3).
	AutonomousAutoContinue    bool `json:"autonomousAutoContinue"`
	AutonomousAutoContinueMax int  `json:"autonomousAutoContinueMax"`

	// FileFreshnessGuard (Claude Code parity). When on, the built-in Edit and Write
	// tools enforce a read-before-write / not-modified-since-read check: an edit (or
	// overwrite of an existing file) errors unless the file was read this session and
	// is unchanged since, so an out-of-band edit is never silently clobbered.
	FileFreshnessGuard bool `json:"fileFreshnessGuard"`

	// AutoTagSessions (event-driven auto-tagging). When on, the runtime derives
	// well-known session tags from turn outcomes + session state — "tool-error" (a
	// real tool failed; a claude-cli disallowed-tool denial is excluded), "error" (the
	// turn itself failed) and "archived" — so an automation can scan
	// and repair them. Add-only (a fixer removes the tag). Detay: _Docs/46 §3.
	AutoTagSessions bool `json:"autoTagSessions"`

	// Per-session debug journal (parallel observability stream). When
	// DebugJournalEnabled is on, the runtime appends structured events (turn
	// timings, token spend, tool latency/size, hook decisions, errors, compaction,
	// recovery) to each session's debug.jsonl for optimisation + agent
	// self-improvement. DebugJournalCap bounds the newest events kept per session
	// (0 = default).
	DebugJournalEnabled bool `json:"debugJournalEnabled"`
	DebugJournalCap     int  `json:"debugJournalCap"`

	// Turn recovery (A1): structural handling of output-token cutoffs and context
	// overflow inside the native agentic tool loop.
	ReactiveCompact    bool `json:"reactiveCompact"`    // fold older history + retry on context overflow
	MaxTokenRetries    int  `json:"maxTokenRetries"`    // resume attempts after the output cap (0 = disabled)
	ReactiveKeepRecent int  `json:"reactiveKeepRecent"` // in-flight messages kept verbatim when compacting
	MaxProviderRetries int  `json:"maxProviderRetries"` // transient provider-fault retries per turn (0 = disabled)
	// Tool-loop guardrail: per-turn loop detection over tool calls. Warnings
	// append recovery guidance to failing tool results; the hard stop
	// additionally blocks repeated identical failures and halts a turn whose
	// tool keeps failing (circuit breaker, opt-in).
	ToolGuardWarnings bool `json:"toolGuardWarnings"`
	ToolGuardHardStop bool `json:"toolGuardHardStop"`
	// Guardrail thresholds (0 = built-in default). Warn thresholds shape the
	// warning hints; block/halt apply only when the hard stop is armed.
	GuardExactWarn      int `json:"guardExactWarn"`       // identical failing call → warn (default 2)
	GuardExactBlock     int `json:"guardExactBlock"`      // identical failing call → block (default 5)
	GuardSameToolWarn   int `json:"guardSameToolWarn"`    // same-tool consecutive failures → warn (default 3)
	GuardSameToolHalt   int `json:"guardSameToolHalt"`    // same-tool consecutive failures → halt turn (default 8)
	GuardNoProgressWarn int `json:"guardNoProgressWarn"`  // identical successful idempotent repeats → warn (default 2)
	GuardNoProgressBlck int `json:"guardNoProgressBlock"` // identical successful idempotent repeats → block (default 5)
	// StuckTurnThreshold: consecutive bad turns (turn error or guardrail halt)
	// before a session is tagged "stuck" and its AUTONOMOUS turns are refused
	// until resolved. 0 disables the gate.
	StuckTurnThreshold int `json:"stuckTurnThreshold"`
	// LessonReflect (hata→ders döngüsü): after a badly-ended turn a background
	// reflection distills the failure into a stored lesson (workspace-wide
	// lessons.jsonl), injected into future turns' dynamic context. One
	// cheap-model call per failing turn.
	LessonReflect bool `json:"lessonReflect"`
	// LessonMaxAgeDays: a lesson whose failure shape does not recur within this
	// many days is pruned as stale (0 = built-in default). Lower = faster decay.
	LessonMaxAgeDays int `json:"lessonMaxAgeDays"`
	// MaxOutputTokens is the generation cap (max output tokens) applied when a
	// turn leaves it unset. 0 = auto: resolve per model family (providers.
	// MaxOutputFor), which keeps answers from being truncated at the providers'
	// 4096 fallback. A positive value pins a fixed global cap across all models
	// (overriding the family table); the TIONHARNESS_MAX_OUTPUT_TOKENS env is the
	// fallback when this is 0.
	MaxOutputTokens int `json:"maxOutputTokens"`

	// Auto-title generation.
	AutoTitleEnabled bool `json:"autoTitleEnabled"`

	// Gated tool capabilities — off by default; each expands agent power/cost.
	// (enableSelfManage was removed 2026-07-01: the self-management suite is always
	// built now; visibility is per-tool.)
	EnableShell    bool `json:"enableShell"`    // built-in shell (arbitrary commands in sandbox)
	EnableCLIHooks bool `json:"enableCliHooks"` // pass PreToolUse/PostToolUse hooks to claude-cli agents via --settings
	// EnableCodeMode (code execution with MCP, _Docs/44): expose the MCP catalog
	// as generated Python bindings behind the run_code tool. Also requires
	// EnableShell (run_code executes arbitrary host code). Native path only.
	EnableCodeMode bool `json:"enableCodeMode"`
	// ClaudeResume keeps the claude-cli session warm across turns: each turn passes
	// --resume <id> and sends only the new turn (not the full transcript), so the
	// CLI reuses its server-side prompt cache (much cheaper, like Claude Code). Off
	// by default — opt-in (the resume id is tracked per session in db.Session).
	ClaudeResume bool `json:"claudeResume"`
	// ClaudePersistentSession keeps ONE long-lived claude-cli process alive per
	// (session, agent) and feeds turns over stdin (stream-json input) instead of
	// spawning a fresh process each turn — warm turns ship only the new user
	// message. Supersedes --resume when on. Default on. _Docs/17.
	ClaudePersistentSession bool `json:"claudePersistentSession"`
	// ClaudeSysPromptFile controls HOW the appended system prompt is handed to the
	// claude-cli subprocess: false (default) passes it inline via
	// --append-system-prompt <text>; true writes it to a temp file and passes
	// --append-system-prompt-file <path>. The file mode sidesteps the Windows ~32 KB
	// command-line limit (errno 206) for very large system prompts; inline is simpler
	// and leaves no temp file behind. _Docs/17.
	ClaudeSysPromptFile bool `json:"claudeSysPromptFile"`
	// run_subagent (isolated subagents / agent→agent delegation) is always installed;
	// availability is managed per-tool from the Tools screen. These remain as
	// per-turn safety guards on every delegation call.
	DelegationMaxDepth int `json:"delegationMaxDepth"` // max subagent nesting (0 = default 3)
	DelegationMaxCalls int `json:"delegationMaxCalls"` // max subagent runs per turn (0 = default 8)

	// Spawn guards — the detached background surface: coordinator workers
	// (spawn_worker), the bridged spawn_session (claude-cli) + the UI spawn button.
	SpawnMaxConcurrent     int `json:"spawnMaxConcurrent"`     // max concurrent spawned sessions (0 = default 16)
	SpawnQueueMax          int `json:"spawnQueueMax"`          // max queued spawned sessions (0 = default 16)
	SpawnMaxPerTurn        int `json:"spawnMaxPerTurn"`        // max spawns per agent turn (0 = default 4)
	SpawnTimeoutMin        int `json:"spawnTimeoutMin"`        // spawn work-turn deadline in minutes (0 = default 20); also budgets its auto-continue continuations
	SpawnIdleTimeoutMin    int `json:"spawnIdleTimeoutMin"`    // spawn/worker inactivity watchdog in minutes (0 = default 5); cancels a turn that emits no step for this long
	ChatTurnTimeoutMin     int `json:"chatTurnTimeoutMin"`     // interactive chat wall-clock ceiling in minutes (0 = disabled)
	ChatTurnIdleTimeoutMin int `json:"chatTurnIdleTimeoutMin"` // interactive chat inactivity window in minutes (0 = disabled; default 3); reclaims a turn whose provider stream stalled
	// CodexStdoutIdleSec is the codex-cli stdout-silence watchdog, in SECONDS: a
	// codex subprocess that has started streaming and then emits NOTHING for this
	// long is killed (whole process tree) and reported as a wedge instead of an
	// empty answer. It must stay BELOW the chat/turn idle watchdogs (whose default
	// is 3 minutes) so the specific, actionable codex diagnosis wins the race
	// against the generic turn cancel. Seconds, not minutes: minute granularity is
	// too coarse to fit under a 3-minute ceiling with any margin
	// (0 = disabled, no stdout-silence watchdog).
	CodexStdoutIdleSec int `json:"codexStdoutIdleSec"`
	IdleResumeMax      int `json:"idleResumeMax"`      // single-shot auto-restarts for an idle-cut background turn (default 1; 0 = disabled)
	ScheduleTimeoutMin int `json:"scheduleTimeoutMin"` // scheduled-fire (cron task/prompt + wake, and the manual "Run now") deadline in minutes (0 = default 60)
	// TurnWatchdogMin is a deprecated absolute limit retained for storage/API
	// compatibility. Active queued-turn cancellation is semantic-idle based
	// (0 = disabled, also the default).
	TurnWatchdogMin int `json:"turnWatchdogMin"`
	// TurnIdleWatchdogMin cancels a queued turn that emits NOTHING (no step, no
	// token) for this long. Wall clock cannot tell a wedged turn from a slow one;
	// silence can, so this is the measure that reclaims a hang quickly while a
	// productive turn remains alive regardless of elapsed wall clock (0 = default 20).
	TurnIdleWatchdogMin int `json:"turnIdleWatchdogMin"`

	// Tool execution guards (process-global tool behaviour).
	ShellDefaultTimeoutSec int `json:"shellDefaultTimeoutSec"` // default Bash/PowerShell timeout in seconds (0 = default 30); per-call timeout_sec still overrides
	ShellMaxTimeoutSec     int `json:"shellMaxTimeoutSec"`     // hard-max Bash/PowerShell timeout in seconds (0 = default 120)
	MaxToolOutputKB        int `json:"maxToolOutputKB"`        // backstop cap on a tool's output in KB before truncation (0 = default 100)
	// AgentMessageMaxKB caps a single agent→agent (send_message) or coordinator→
	// worker (send_to_worker) message body. Over the cap the call fails with an
	// explicit message_too_large error; the one-way worker notification path is
	// capped with a visible marker instead (0 = default 64). _Docs/47.
	AgentMessageMaxKB int `json:"agentMessageMaxKB"`

	// Coordinator/worker guards (M2, _Docs/47).
	CoordinatorMaxWorkers int `json:"coordinatorMaxWorkers"` // max active workers per coordinator (0 = default 8)
	CoordinatorMaxTurns   int `json:"coordinatorMaxTurns"`   // max auto-triggered coordinator turns per session (0 = default 50, -1 = unlimited)
	// CoordinatorMaxDepth bounds how deep a coordinator TREE may nest (root = 0);
	// -1 = unlimited nesting. CoordinatorMaxSubtreeSessions bounds the TOTAL worker
	// sessions in one tree across every level; -1 = unlimited. The per-coordinator
	// worker cap above cannot do that job: it is enforced per node, so depth
	// multiplies it instead of adding to it.
	CoordinatorMaxDepth           int `json:"coordinatorMaxDepth"`
	CoordinatorMaxSubtreeSessions int `json:"coordinatorMaxSubtreeSessions"`
	// CoordinatorSettleGraceSec is how long the upward-report backstop waits after
	// a sub-coordinator's branch goes quiet before auto-reporting for it. Tune it to
	// the model behind the coordinators: too short and a slow synthesis turn loses
	// the race, so the parent gets a needless "incomplete"; too long and a genuinely
	// stalled branch keeps its coordinator waiting.
	CoordinatorSettleGraceSec int `json:"coordinatorSettleGraceSec"`

	// Coordinator stall/hallucination protection (see internal/agent/coordination_stall.go).
	// CoordinatorStallGuard is the master switch for the judge-based turn-end guard +
	// staleness sweeper that catch a coordinator narrating a spawn it never issued
	// (default true). CoordinatorStallSweepMin is the staleness window in minutes
	// (0 = default 5; negative disables the sweeper while leaving the turn-end guard
	// on). CoordinatorStallMaxNudges caps consecutive corrective nudges (0 = default 2).
	CoordinatorStallGuard     bool `json:"coordinatorStallGuard"`
	CoordinatorStallSweepMin  int  `json:"coordinatorStallSweepMin"`
	CoordinatorStallMaxNudges int  `json:"coordinatorStallMaxNudges"`

	// Working-directory guards. The built-in fs/shell tools are unconfined (may
	// touch any path); these brake that power on autonomous (no-human) turns.
	// Scope caveat: AutonomousConfine confines the FS tools, but shell/powershell
	// are NOT path-restricted by it. transform_data resolves its input/output
	// arguments through the sandbox, but its arbitrary host-code script can bypass
	// that path restriction. See internal/tools/builtin_shell.go.
	AutonomousConfine bool `json:"autonomousConfine"` // confine the fs tools to the working dir on autonomous turns; shell is unaffected (default true)
	AutonomousBootSeq bool `json:"autonomousBootSeq"` // inject the boot/verification-sequence reminder on autonomous turns (default true)

	// Workspace backups — periodic, retention-bounded zip snapshots of every
	// workspace's data directory. Off by default.
	BackupEnabled       bool   `json:"backupEnabled"`
	BackupIntervalHours int    `json:"backupIntervalHours"` // hours between automatic runs (min 1)
	BackupRetain        int    `json:"backupRetain"`        // newest archives kept per workspace (min 1)
	BackupDir           string `json:"backupDir"`           // backups root; "" → <dataDir>/backups
}

// Default returns the baseline settings used when no file exists yet. Values
// mirror the historical env/const defaults so behaviour is unchanged until the
// user edits anything.
func Default() Settings {
	return Settings{
		Theme:       ThemeDark,
		Accent:      "#8b5cf6",
		ThemePreset: "violet-dark",
		Language:    DefaultLanguage,
		UILanguage:  "", // follow Language until the user picks an interface language

		DefaultPermissionMode: "auto",
		ClaudeCLIPath:         "",
		ClaudeConfigDir:       defaultClaudeConfigDir(),
		ClaudeCliAuthKind:     "",
		CodexCLIPath:          "",
		CodexConfigDir:        defaultCodexConfigDir(),

		MaxContextTokens: 800000,
		KeepRecentMsgs:   8,
		// Context-rot-aware default (2026-06-25, _Docs/17 §12): ceil 256K keeps the
		// live window in the gradient's high-precision zone; fraction 0 = "auto"
		// (per-family adaptive). Durability of folded detail comes from retrieval
		// (memory / conversation_search / core blocks), not from a huge raw window.
		ContextBudgetCeil:     262144,
		ContextBudgetFraction: 0,
		// Rolling keeps the pre-existing fold on upgrade; native/auto are opt-in.
		AutoCompactMode: AutoCompactRolling,

		// Context reset / handoff: off by default; the manual /handoff command and the
		// handoff_session tool work regardless. Defaults match agent.DefaultHandoff*.
		HandoffAuto:      false,
		HandoffMaxChain:  20,
		HandoffWriteFile: false,

		// Persistent progress: on by default — only adds a per-project file and is
		// transparent to existing behaviour.
		ProgressPersist: true,
		ProgressResume:  true,

		// Autonomous self-completion: on by default so unattended (scheduler/spawn/
		// wake) turns that stall after activating tools or with open todos continue
		// themselves instead of leaving the work half-done. Bounded at 3 turns.
		AutonomousAutoContinue:    true,
		AutonomousAutoContinueMax: 3,

		// File freshness guard on by default (Claude Code parity): Edit/Write refuse to
		// clobber a file changed out-of-band since it was last read.
		FileFreshnessGuard: true,

		// Extended prompt caching (1h TTL) on by default: cache reads bill at
		// ~0.1× the input price, so with the static/dynamic prompt split every
		// follow-up turn re-reads the transcript prefix nearly free instead of
		// paying full input price. Only affects the anthropic provider; existing
		// settings.json files keep whatever the user last saved.
		ExtendedPromptCache: true,

		// Refusal fallback on by default (Anthropic's Fable 5 guidance): only
		// Fable-class requests carry it, and without it a classifier false
		// positive fails the turn outright.
		AnthropicRefusalFallback: true,

		// Auto-tagging on by default: derives error/archived tags for automation
		// scanning; add-only and cheap (a small write only when a tag actually changes).
		AutoTagSessions: true,

		// Debug journal on by default: only adds a per-session file, transparent to
		// existing behaviour. 5000 newest events kept per session.
		DebugJournalEnabled: true,
		DebugJournalCap:     5000,

		ReactiveCompact:     true,
		MaxTokenRetries:     3,
		ReactiveKeepRecent:  6,
		MaxProviderRetries:  2,
		ToolGuardWarnings:   true,
		ToolGuardHardStop:   false,
		GuardExactWarn:      2,
		GuardExactBlock:     5,
		GuardSameToolWarn:   3,
		GuardSameToolHalt:   8,
		GuardNoProgressWarn: 2,
		GuardNoProgressBlck: 5,
		StuckTurnThreshold:  3,
		LessonReflect:       true,
		LessonMaxAgeDays:    2,
		MaxOutputTokens:     0, // auto: per-model family default

		AutoTitleEnabled: true,

		// CLI-path hooks default ON (preserves the hook-passthrough behaviour); turn
		// off when a hook authored for TionHarness's shell misbehaves under the CLI's.
		EnableCLIHooks: true,

		// claude-cli session resume default ON: each turn passes --resume <id> and
		// sends only the delta, so the CLI reuses its warm server-side prompt cache
		// (cache_read instead of a full cold cache write). Combined with the static/
		// dynamic prompt split (see providers.ClaudeCLI.buildSystemAndPrompt), this
		// keeps the cached prefix warm turn-to-turn. _Docs/17.
		ClaudeResume: true,

		// Persistent claude-cli process default ON: keeps ONE warm process per
		// (session, agent) fed over stdin, so warm turns ship only the new message
		// and the process holds the rest. Supersedes --resume when both are on.
		// _Docs/17.
		ClaudePersistentSession: true,

		// System prompt handed to claude-cli via a temp file by default
		// (--append-system-prompt-file): a large appended prompt (skills + lazy tool
		// catalog + dynamic context) as an inline --append-system-prompt argument can
		// overflow the Windows ~32 KB command-line limit and crash fork/exec. The file
		// carries only a short path, so it is robust regardless of prompt size. Flip
		// off for the inline form only if a platform lacks temp-file access.
		ClaudeSysPromptFile: true,

		DelegationMaxDepth: 3,
		DelegationMaxCalls: 8,

		SpawnMaxConcurrent:     16,
		SpawnQueueMax:          16,
		SpawnMaxPerTurn:        4,
		SpawnTimeoutMin:        0,
		SpawnIdleTimeoutMin:    5,
		ChatTurnTimeoutMin:     0,
		ChatTurnIdleTimeoutMin: 3,
		// 90 seconds: longer than a normal quiet gap inside a codex turn (a tool call
		// that prints nothing while it works), yet comfortably under the 3-minute
		// chat/turn idle watchdogs — so a wedged codex subprocess is diagnosed and
		// killed HERE, with its stdout tail, instead of being swallowed by the
		// generic turn cancel that would otherwise always fire first.
		CodexStdoutIdleSec:  90,
		IdleResumeMax:       1,
		ScheduleTimeoutMin:  0,
		TurnWatchdogMin:     0,
		TurnIdleWatchdogMin: 20,

		ShellDefaultTimeoutSec: 30,
		ShellMaxTimeoutSec:     120,
		MaxToolOutputKB:        100,
		AgentMessageMaxKB:      64,

		CoordinatorMaxWorkers:         8,
		CoordinatorMaxTurns:           -1, // unlimited (not user-configurable; see normalize)
		CoordinatorMaxDepth:           5,
		CoordinatorMaxSubtreeSessions: -1, // unlimited (not user-configurable; see normalize)
		CoordinatorSettleGraceSec:     30,
		CoordinatorStallGuard:         true,
		CoordinatorStallSweepMin:      5,
		CoordinatorStallMaxNudges:     2,

		// Autonomous turns confine fs/shell by default (safety brake).
		AutonomousConfine: true,
		AutonomousBootSeq: true,

		// Workspace backups off by default; daily cadence, keep a week of snapshots.
		BackupEnabled:       false,
		BackupIntervalHours: 24,
		BackupRetain:        7,
		BackupDir:           "",
	}
}

// DTO is the client-facing view of settings: identical to Settings minus the
// encrypted secret, plus a boolean reporting whether a key is configured.
type DTO struct {
	Theme       string `json:"theme"`
	Accent      string `json:"accent"`
	ThemePreset string `json:"themePreset"`
	Language    string `json:"language"`
	UILanguage  string `json:"uiLanguage"`

	DefaultPermissionMode string `json:"defaultPermissionMode"`
	ClaudeConfigDir       string `json:"claudeConfigDir"`
	ClaudeCliAuthKind     string `json:"claudeCliAuthKind"`
	ClaudeCliAuthSet      bool   `json:"claudeCliAuthSet"`
	// AnthropicKeySet reflects AnthropicKeyEnc, which stays live: it seeds the
	// default "anthropic" provider instance from ANTHROPIC_API_KEY at boot
	// (internal/app/app.go) and from a manual key entry via Patch.AnthropicKey,
	// which app.go also calls internally at boot — not just an API-facing field.
	// Every other legacy typed provider field (Minimax/OpenRouter/ZAI/DeepSeek/
	// CustomProviders/ClaudeCLIPath/CodexCLIPath/CodexConfigDir) was removed
	// from the DTO/Patch: they have no live reader/writer left outside
	// MigrateFromSettings, which
	// reads the underlying Settings struct fields directly (kept for that
	// one-time boot migration; never exposed to the API).
	AnthropicKeySet bool `json:"anthropicKeySet"`

	ExtendedPromptCache        bool `json:"extendedPromptCache"`
	AnthropicContextEditing    bool `json:"anthropicContextEditing"`
	AnthropicNativeToolSearch  bool `json:"anthropicNativeToolSearch"`
	AnthropicProgrammaticTools bool `json:"anthropicProgrammaticTools"`
	AnthropicWebTools          bool `json:"anthropicWebTools"`
	AnthropicServerCompaction  bool `json:"anthropicServerCompaction"`
	AnthropicRefusalFallback   bool `json:"anthropicRefusalFallback"`
	AutonomousTaskBudgetTokens int  `json:"autonomousTaskBudgetTokens"`

	DesktopNotifications bool `json:"desktopNotifications"`
	KeepAwake            bool `json:"keepAwake"`

	UserName     string `json:"userName"`
	UserTimezone string `json:"userTimezone"`
	UserCity     string `json:"userCity"`
	UserCountry  string `json:"userCountry"`
	UserNotes    string `json:"userNotes"`

	MaxContextTokens int `json:"maxContextTokens"`
	KeepRecentMsgs   int `json:"keepRecentMsgs"`

	ContextBudgetCeil     int     `json:"contextBudgetCeil"`
	ContextBudgetFraction float64 `json:"contextBudgetFraction"`

	AutoCompactMode string `json:"autoCompactMode"`

	HandoffAuto      bool `json:"handoffAuto"`
	HandoffMaxChain  int  `json:"handoffMaxChain"`
	HandoffWriteFile bool `json:"handoffWriteFile"`

	ProgressPersist bool `json:"progressPersist"`
	ProgressResume  bool `json:"progressResume"`

	AutonomousAutoContinue    bool `json:"autonomousAutoContinue"`
	AutonomousAutoContinueMax int  `json:"autonomousAutoContinueMax"`

	FileFreshnessGuard bool `json:"fileFreshnessGuard"`
	AutoTagSessions    bool `json:"autoTagSessions"`

	DebugJournalEnabled bool `json:"debugJournalEnabled"`
	DebugJournalCap     int  `json:"debugJournalCap"`

	ReactiveCompact     bool `json:"reactiveCompact"`
	MaxTokenRetries     int  `json:"maxTokenRetries"`
	ReactiveKeepRecent  int  `json:"reactiveKeepRecent"`
	MaxProviderRetries  int  `json:"maxProviderRetries"`
	ToolGuardWarnings   bool `json:"toolGuardWarnings"`
	ToolGuardHardStop   bool `json:"toolGuardHardStop"`
	GuardExactWarn      int  `json:"guardExactWarn"`
	GuardExactBlock     int  `json:"guardExactBlock"`
	GuardSameToolWarn   int  `json:"guardSameToolWarn"`
	GuardSameToolHalt   int  `json:"guardSameToolHalt"`
	GuardNoProgressWarn int  `json:"guardNoProgressWarn"`
	GuardNoProgressBlck int  `json:"guardNoProgressBlock"`
	StuckTurnThreshold  int  `json:"stuckTurnThreshold"`
	LessonReflect       bool `json:"lessonReflect"`
	LessonMaxAgeDays    int  `json:"lessonMaxAgeDays"`
	MaxOutputTokens     int  `json:"maxOutputTokens"`

	AutoTitleEnabled bool `json:"autoTitleEnabled"`

	EnableShell    bool `json:"enableShell"`
	EnableCLIHooks bool `json:"enableCliHooks"`
	EnableCodeMode bool `json:"enableCodeMode"`
	ClaudeResume   bool `json:"claudeResume"`
	// ClaudePersistentSession keeps ONE long-lived claude-cli process alive per
	// (session, agent) and feeds turns over stdin (stream-json input) instead of
	// spawning a fresh process each turn. The process holds the conversation
	// in-memory so warm turns ship only the new user message — maximal prompt-cache
	// reuse + no per-turn startup. Supersedes --resume when on. Default on. _Docs/17.
	ClaudePersistentSession bool `json:"claudePersistentSession"`
	// ClaudeSysPromptFile: true (default) routes the appended system prompt through a
	// temp file (--append-system-prompt-file) to survive the Windows command-line
	// limit; false hands it inline via --append-system-prompt. _Docs/17.
	ClaudeSysPromptFile bool `json:"claudeSysPromptFile"`
	DelegationMaxDepth  int  `json:"delegationMaxDepth"`
	DelegationMaxCalls  int  `json:"delegationMaxCalls"`

	SpawnMaxConcurrent     int `json:"spawnMaxConcurrent"`
	SpawnQueueMax          int `json:"spawnQueueMax"`
	SpawnMaxPerTurn        int `json:"spawnMaxPerTurn"`
	SpawnTimeoutMin        int `json:"spawnTimeoutMin"`
	SpawnIdleTimeoutMin    int `json:"spawnIdleTimeoutMin"`
	ChatTurnTimeoutMin     int `json:"chatTurnTimeoutMin"`
	ChatTurnIdleTimeoutMin int `json:"chatTurnIdleTimeoutMin"`
	CodexStdoutIdleSec     int `json:"codexStdoutIdleSec"`
	IdleResumeMax          int `json:"idleResumeMax"`
	ScheduleTimeoutMin     int `json:"scheduleTimeoutMin"`
	TurnWatchdogMin        int `json:"turnWatchdogMin"`
	TurnIdleWatchdogMin    int `json:"turnIdleWatchdogMin"`

	ShellDefaultTimeoutSec int `json:"shellDefaultTimeoutSec"`
	ShellMaxTimeoutSec     int `json:"shellMaxTimeoutSec"`
	MaxToolOutputKB        int `json:"maxToolOutputKB"`
	AgentMessageMaxKB      int `json:"agentMessageMaxKB"`

	CoordinatorMaxWorkers         int  `json:"coordinatorMaxWorkers"`
	CoordinatorMaxTurns           int  `json:"coordinatorMaxTurns"`
	CoordinatorMaxDepth           int  `json:"coordinatorMaxDepth"`
	CoordinatorMaxSubtreeSessions int  `json:"coordinatorMaxSubtreeSessions"`
	CoordinatorSettleGraceSec     int  `json:"coordinatorSettleGraceSec"`
	CoordinatorStallGuard         bool `json:"coordinatorStallGuard"`
	CoordinatorStallSweepMin      int  `json:"coordinatorStallSweepMin"`
	CoordinatorStallMaxNudges     int  `json:"coordinatorStallMaxNudges"`

	AutonomousConfine bool `json:"autonomousConfine"`
	AutonomousBootSeq bool `json:"autonomousBootSeq"`

	BackupEnabled       bool   `json:"backupEnabled"`
	BackupIntervalHours int    `json:"backupIntervalHours"`
	BackupRetain        int    `json:"backupRetain"`
	BackupDir           string `json:"backupDir"`
}

// ToDTO projects persisted settings into the client view, masking the secret.
func (s Settings) ToDTO() DTO {
	return DTO{
		Theme:       s.Theme,
		Accent:      s.Accent,
		ThemePreset: s.ThemePreset,
		Language:    s.Language,
		UILanguage:  s.UILanguage,

		DefaultPermissionMode: s.DefaultPermissionMode,
		ClaudeConfigDir:       s.ClaudeConfigDir,
		ClaudeCliAuthKind:     s.ClaudeCliAuthKind,
		ClaudeCliAuthSet:      s.ClaudeCliAuthTokenEnc != "",
		AnthropicKeySet:       s.AnthropicKeyEnc != "",

		ExtendedPromptCache:        s.ExtendedPromptCache,
		AnthropicContextEditing:    s.AnthropicContextEditing,
		AnthropicNativeToolSearch:  s.AnthropicNativeToolSearch,
		AnthropicProgrammaticTools: s.AnthropicProgrammaticTools,
		AnthropicWebTools:          s.AnthropicWebTools,
		AnthropicServerCompaction:  s.AnthropicServerCompaction,
		AnthropicRefusalFallback:   s.AnthropicRefusalFallback,
		AutonomousTaskBudgetTokens: s.AutonomousTaskBudgetTokens,

		DesktopNotifications: s.DesktopNotifications,
		KeepAwake:            s.KeepAwake,

		UserName:     s.UserName,
		UserTimezone: s.UserTimezone,
		UserCity:     s.UserCity,
		UserCountry:  s.UserCountry,
		UserNotes:    s.UserNotes,

		MaxContextTokens: s.MaxContextTokens,
		KeepRecentMsgs:   s.KeepRecentMsgs,

		ContextBudgetCeil:     s.ContextBudgetCeil,
		ContextBudgetFraction: s.ContextBudgetFraction,

		AutoCompactMode: s.AutoCompactMode,

		HandoffAuto:      s.HandoffAuto,
		HandoffMaxChain:  s.HandoffMaxChain,
		HandoffWriteFile: s.HandoffWriteFile,

		ProgressPersist: s.ProgressPersist,
		ProgressResume:  s.ProgressResume,

		AutonomousAutoContinue:    s.AutonomousAutoContinue,
		AutonomousAutoContinueMax: s.AutonomousAutoContinueMax,

		FileFreshnessGuard: s.FileFreshnessGuard,
		AutoTagSessions:    s.AutoTagSessions,

		DebugJournalEnabled: s.DebugJournalEnabled,
		DebugJournalCap:     s.DebugJournalCap,

		ReactiveCompact:     s.ReactiveCompact,
		MaxTokenRetries:     s.MaxTokenRetries,
		ReactiveKeepRecent:  s.ReactiveKeepRecent,
		MaxProviderRetries:  s.MaxProviderRetries,
		ToolGuardWarnings:   s.ToolGuardWarnings,
		ToolGuardHardStop:   s.ToolGuardHardStop,
		GuardExactWarn:      s.GuardExactWarn,
		GuardExactBlock:     s.GuardExactBlock,
		GuardSameToolWarn:   s.GuardSameToolWarn,
		GuardSameToolHalt:   s.GuardSameToolHalt,
		GuardNoProgressWarn: s.GuardNoProgressWarn,
		GuardNoProgressBlck: s.GuardNoProgressBlck,
		StuckTurnThreshold:  s.StuckTurnThreshold,
		LessonReflect:       s.LessonReflect,
		LessonMaxAgeDays:    s.LessonMaxAgeDays,
		MaxOutputTokens:     s.MaxOutputTokens,

		AutoTitleEnabled: s.AutoTitleEnabled,

		EnableShell:             s.EnableShell,
		EnableCLIHooks:          s.EnableCLIHooks,
		EnableCodeMode:          s.EnableCodeMode,
		ClaudeResume:            s.ClaudeResume,
		ClaudePersistentSession: s.ClaudePersistentSession,
		ClaudeSysPromptFile:     s.ClaudeSysPromptFile,
		DelegationMaxDepth:      s.DelegationMaxDepth,
		DelegationMaxCalls:      s.DelegationMaxCalls,

		SpawnMaxConcurrent:     s.SpawnMaxConcurrent,
		SpawnQueueMax:          s.SpawnQueueMax,
		SpawnMaxPerTurn:        s.SpawnMaxPerTurn,
		SpawnTimeoutMin:        s.SpawnTimeoutMin,
		SpawnIdleTimeoutMin:    s.SpawnIdleTimeoutMin,
		ChatTurnTimeoutMin:     s.ChatTurnTimeoutMin,
		ChatTurnIdleTimeoutMin: s.ChatTurnIdleTimeoutMin,
		CodexStdoutIdleSec:     s.CodexStdoutIdleSec,
		IdleResumeMax:          s.IdleResumeMax,
		ScheduleTimeoutMin:     s.ScheduleTimeoutMin,
		TurnWatchdogMin:        s.TurnWatchdogMin,
		TurnIdleWatchdogMin:    s.TurnIdleWatchdogMin,

		ShellDefaultTimeoutSec: s.ShellDefaultTimeoutSec,
		ShellMaxTimeoutSec:     s.ShellMaxTimeoutSec,
		MaxToolOutputKB:        s.MaxToolOutputKB,
		AgentMessageMaxKB:      s.AgentMessageMaxKB,

		CoordinatorMaxWorkers:         s.CoordinatorMaxWorkers,
		CoordinatorMaxTurns:           s.CoordinatorMaxTurns,
		CoordinatorMaxDepth:           s.CoordinatorMaxDepth,
		CoordinatorMaxSubtreeSessions: s.CoordinatorMaxSubtreeSessions,
		CoordinatorSettleGraceSec:     s.CoordinatorSettleGraceSec,
		CoordinatorStallGuard:         s.CoordinatorStallGuard,
		CoordinatorStallSweepMin:      s.CoordinatorStallSweepMin,
		CoordinatorStallMaxNudges:     s.CoordinatorStallMaxNudges,

		AutonomousConfine: s.AutonomousConfine,
		AutonomousBootSeq: s.AutonomousBootSeq,

		BackupEnabled:       s.BackupEnabled,
		BackupIntervalHours: s.BackupIntervalHours,
		BackupRetain:        s.BackupRetain,
		BackupDir:           s.BackupDir,
	}
}

// Patch is a partial update: every field is a pointer so the client can change
// any subset. AnthropicKey is write-only — a non-nil empty string clears the
// stored key, a non-empty value replaces it.
type Patch struct {
	Theme       *string `json:"theme"`
	Accent      *string `json:"accent"`
	ThemePreset *string `json:"themePreset"`
	Language    *string `json:"language"`
	UILanguage  *string `json:"uiLanguage"`

	DefaultPermissionMode *string `json:"defaultPermissionMode"`
	ClaudeConfigDir       *string `json:"claudeConfigDir"`
	ClaudeCliAuthKind     *string `json:"claudeCliAuthKind"`
	ClaudeCliAuthToken    *string `json:"claudeCliAuthToken"` // write-only
	AnthropicKey          *string `json:"anthropicKey"`       // write-only

	ExtendedPromptCache        *bool `json:"extendedPromptCache"`
	AnthropicContextEditing    *bool `json:"anthropicContextEditing"`
	AnthropicNativeToolSearch  *bool `json:"anthropicNativeToolSearch"`
	AnthropicProgrammaticTools *bool `json:"anthropicProgrammaticTools"`
	AnthropicRefusalFallback   *bool `json:"anthropicRefusalFallback"`
	AnthropicWebTools          *bool `json:"anthropicWebTools"`
	AnthropicServerCompaction  *bool `json:"anthropicServerCompaction"`
	AutonomousTaskBudgetTokens *int  `json:"autonomousTaskBudgetTokens"`

	DesktopNotifications *bool `json:"desktopNotifications"`
	KeepAwake            *bool `json:"keepAwake"`

	UserName     *string `json:"userName"`
	UserTimezone *string `json:"userTimezone"`
	UserCity     *string `json:"userCity"`
	UserCountry  *string `json:"userCountry"`
	UserNotes    *string `json:"userNotes"`

	MaxContextTokens *int `json:"maxContextTokens"`
	KeepRecentMsgs   *int `json:"keepRecentMsgs"`

	ContextBudgetCeil     *int     `json:"contextBudgetCeil"`
	ContextBudgetFraction *float64 `json:"contextBudgetFraction"`

	AutoCompactMode *string `json:"autoCompactMode"`

	HandoffAuto      *bool `json:"handoffAuto"`
	HandoffMaxChain  *int  `json:"handoffMaxChain"`
	HandoffWriteFile *bool `json:"handoffWriteFile"`

	ProgressPersist *bool `json:"progressPersist"`
	ProgressResume  *bool `json:"progressResume"`

	AutonomousAutoContinue    *bool `json:"autonomousAutoContinue"`
	AutonomousAutoContinueMax *int  `json:"autonomousAutoContinueMax"`

	FileFreshnessGuard *bool `json:"fileFreshnessGuard"`
	AutoTagSessions    *bool `json:"autoTagSessions"`

	DebugJournalEnabled *bool `json:"debugJournalEnabled"`
	DebugJournalCap     *int  `json:"debugJournalCap"`

	ReactiveCompact     *bool `json:"reactiveCompact"`
	MaxTokenRetries     *int  `json:"maxTokenRetries"`
	ReactiveKeepRecent  *int  `json:"reactiveKeepRecent"`
	MaxProviderRetries  *int  `json:"maxProviderRetries"`
	ToolGuardWarnings   *bool `json:"toolGuardWarnings"`
	ToolGuardHardStop   *bool `json:"toolGuardHardStop"`
	GuardExactWarn      *int  `json:"guardExactWarn"`
	GuardExactBlock     *int  `json:"guardExactBlock"`
	GuardSameToolWarn   *int  `json:"guardSameToolWarn"`
	GuardSameToolHalt   *int  `json:"guardSameToolHalt"`
	GuardNoProgressWarn *int  `json:"guardNoProgressWarn"`
	GuardNoProgressBlck *int  `json:"guardNoProgressBlock"`
	StuckTurnThreshold  *int  `json:"stuckTurnThreshold"`
	LessonReflect       *bool `json:"lessonReflect"`
	LessonMaxAgeDays    *int  `json:"lessonMaxAgeDays"`
	MaxOutputTokens     *int  `json:"maxOutputTokens"`

	AutoTitleEnabled *bool `json:"autoTitleEnabled"`

	EnableShell             *bool `json:"enableShell"`
	EnableCLIHooks          *bool `json:"enableCliHooks"`
	EnableCodeMode          *bool `json:"enableCodeMode"`
	ClaudeResume            *bool `json:"claudeResume"`
	ClaudePersistentSession *bool `json:"claudePersistentSession"`
	ClaudeSysPromptFile     *bool `json:"claudeSysPromptFile"`
	DelegationMaxDepth      *int  `json:"delegationMaxDepth"`
	DelegationMaxCalls      *int  `json:"delegationMaxCalls"`

	SpawnMaxConcurrent     *int `json:"spawnMaxConcurrent"`
	SpawnQueueMax          *int `json:"spawnQueueMax"`
	SpawnMaxPerTurn        *int `json:"spawnMaxPerTurn"`
	SpawnTimeoutMin        *int `json:"spawnTimeoutMin"`
	SpawnIdleTimeoutMin    *int `json:"spawnIdleTimeoutMin"`
	ChatTurnTimeoutMin     *int `json:"chatTurnTimeoutMin"`
	ChatTurnIdleTimeoutMin *int `json:"chatTurnIdleTimeoutMin"`
	CodexStdoutIdleSec     *int `json:"codexStdoutIdleSec"`
	IdleResumeMax          *int `json:"idleResumeMax"`
	ScheduleTimeoutMin     *int `json:"scheduleTimeoutMin"`
	TurnWatchdogMin        *int `json:"turnWatchdogMin"`
	TurnIdleWatchdogMin    *int `json:"turnIdleWatchdogMin"`

	ShellDefaultTimeoutSec *int `json:"shellDefaultTimeoutSec"`
	ShellMaxTimeoutSec     *int `json:"shellMaxTimeoutSec"`
	MaxToolOutputKB        *int `json:"maxToolOutputKB"`
	AgentMessageMaxKB      *int `json:"agentMessageMaxKB"`

	CoordinatorMaxWorkers         *int  `json:"coordinatorMaxWorkers"`
	CoordinatorMaxTurns           *int  `json:"coordinatorMaxTurns"`
	CoordinatorMaxDepth           *int  `json:"coordinatorMaxDepth"`
	CoordinatorMaxSubtreeSessions *int  `json:"coordinatorMaxSubtreeSessions"`
	CoordinatorSettleGraceSec     *int  `json:"coordinatorSettleGraceSec"`
	CoordinatorStallGuard         *bool `json:"coordinatorStallGuard"`
	CoordinatorStallSweepMin      *int  `json:"coordinatorStallSweepMin"`
	CoordinatorStallMaxNudges     *int  `json:"coordinatorStallMaxNudges"`

	AutonomousConfine *bool `json:"autonomousConfine"`
	AutonomousBootSeq *bool `json:"autonomousBootSeq"`

	BackupEnabled       *bool   `json:"backupEnabled"`
	BackupIntervalHours *int    `json:"backupIntervalHours"`
	BackupRetain        *int    `json:"backupRetain"`
	BackupDir           *string `json:"backupDir"`
}
