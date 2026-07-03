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
// SwarmGo-managed, isolated config home (~/.swarmgo/claude-home) so the CLI runs
// against a clean skills/settings/commands/login set instead of the user's shared
// ~/.claude out of the box. Requires a one-time `claude` login in that directory.
// If the home dir can't be resolved we return "" — that is the correct fallback
// (inherit the ambient ~/.claude), not a swallowed error.
func defaultClaudeConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".swarmgo", "claude-home")
}

// Theme options for the UI.
const (
	ThemeDark   = "dark"
	ThemeLight  = "light"
	ThemeSystem = "system"
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

// CustomProviderDTO is the masked, client-facing view of a CustomProvider.
type CustomProviderDTO struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	BaseURL      string `json:"baseUrl"`
	DefaultModel string `json:"defaultModel"`
	Models       string `json:"models"`
	KeySet       bool   `json:"keySet"`
	Reasoning    bool   `json:"reasoning,omitempty"`
	PromptCache  string `json:"promptCache,omitempty"`
}

// Settings is the full, persisted configuration document. The encrypted
// Anthropic key lives in AnthropicKeyEnc and is never serialized to the API
// (json tag "-"); clients see only AnthropicKeySet via the DTO.
type Settings struct {
	// Appearance.
	Theme       string `json:"theme"`
	Accent      string `json:"accent"`      // hex color, e.g. "#4f8cff"
	ThemePreset string `json:"themePreset"` // theme color + variant id, e.g. "violet-dark" ("" = default)
	Language    string `json:"language"`    // "tr" | "en"

	// Providers.
	DefaultProvider string `json:"defaultProvider"` // "claude-cli" | "anthropic"
	DefaultModel    string `json:"defaultModel"`    // "" = provider default
	// DefaultPermissionMode seeds new agents' tool-use permission gate:
	// "read-only" | "ask" | "auto". "" falls back to "auto".
	DefaultPermissionMode string `json:"defaultPermissionMode"`
	ClaudeCLIPath         string `json:"claudeCliPath"`   // "" = auto-detect on PATH
	ClaudeConfigDir       string `json:"claudeConfigDir"` // CLAUDE_CONFIG_DIR for claude-cli; "" = inherit ~/.claude
	// claude-cli credential injected into the subprocess env so an isolated config
	// dir authenticates without an interactive in-dir `claude login`. The token is
	// AES-GCM encrypted (never serialized to the API); the kind selects the env var:
	//   "oauth"  → CLAUDE_CODE_OAUTH_TOKEN (Max/Pro subscription, from `claude setup-token`)
	//   "apikey" → ANTHROPIC_API_KEY       (API billing)
	//   ""       → none injected (rely on the config dir's own login)
	ClaudeCliAuthKind     string `json:"claudeCliAuthKind"`
	ClaudeCliAuthTokenEnc string `json:"claudeCliAuthTokenEnc"` // AES-GCM, never exposed
	AnthropicKeyEnc       string `json:"anthropicKeyEnc"`       // AES-GCM, never exposed

	// MiniMax (OpenAI-compatible) provider.
	MinimaxKeyEnc  string `json:"minimaxKeyEnc"` // AES-GCM, never exposed
	MinimaxBaseURL string `json:"minimaxBaseUrl"`

	// OpenRouter (OpenAI-compatible) provider — one key, hundreds of models.
	OpenRouterKeyEnc  string `json:"openrouterKeyEnc"` // AES-GCM, never exposed
	OpenRouterBaseURL string `json:"openrouterBaseUrl"`

	// CustomProviders are user-added OpenAI- or Anthropic-compatible endpoints
	// (OpenRouter, Gemini, Kimi, Ollama, ...). Each is selectable as a provider
	// id alongside the built-ins; the key is AES-GCM encrypted like the others.
	CustomProviders []CustomProvider `json:"customProviders"`

	// Anthropic beta capabilities (anthropic provider only; claude-cli ignores).
	// (The 1M-context beta was retired — 1M is GA since 2026-03, no toggle needed.)
	ExtendedPromptCache bool `json:"extendedPromptCache"` // 1h extended prompt cache TTL

	// Desktop / display behaviour (applied client-side).
	DesktopNotifications bool `json:"desktopNotifications"` // browser notifications
	KeepAwake            bool `json:"keepAwake"`            // hold a screen wake lock

	// User profile — injected as context so agents address the user correctly.
	UserName     string `json:"userName"`
	UserTimezone string `json:"userTimezone"`
	UserCity     string `json:"userCity"`
	UserCountry  string `json:"userCountry"`
	UserNotes    string `json:"userNotes"`

	// Context & memory.
	MaxContextTokens int     `json:"maxContextTokens"`
	KeepRecentMsgs   int     `json:"keepRecentMsgs"`
	RecallTopN       int     `json:"recallTopN"`
	RecallMinScore   float64 `json:"recallMinScore"`
	// Model-aware transcript budget (see internal/conversation/budget.go). The live
	// budget is clamp(window * ContextBudgetFraction, MaxContextTokens, ContextBudgetCeil)
	// when the model's context window is known. ContextBudgetFraction = 0 means "auto"
	// → a context-rot-aware per-family share (providers.AdaptiveBudgetFraction); a
	// positive value pins a fixed manual share. ContextBudgetCeil is the operative cap
	// for big-window (1M) models — raise it to keep more history verbatim before the
	// first silent compaction (trades recall precision for raw history, _Docs/17 §12).
	ContextBudgetCeil     int     `json:"contextBudgetCeil"`
	ContextBudgetFraction float64 `json:"contextBudgetFraction"`

	// Journal (long-term memory) ring-buffer bounds.
	JournalCap    int `json:"journalCap"`    // newest journal entries kept per agent (0 = default)
	JournalMaxLen int `json:"journalMaxLen"` // max runes stored per journal entry (0 = default)
	// JournalMinLen is the write-side low-info gate: a per-turn journal whose
	// content is shorter than this many runes is dropped instead of stored, so
	// trivial exchanges (a one-word/one-number answer) never enter the recall
	// pool and waste fresh tokens in the uncached dynamic context every turn.
	// 0 disables the gate (every non-empty turn is journaled).
	JournalMinLen int `json:"journalMinLen"`
	ReflectionCap int `json:"reflectionCap"` // newest reflections kept per agent; older pruned each dream cycle (0 = default)

	// MemGPT-style self-editing memory (C6). MemoryPressureWarn is the context-fill
	// ratio (0..1) above which a turn warns the agent to persist important facts
	// before the next silent compaction; 0 disables the warning. CoreMemoryTools
	// offers the core_memory_replace/append editing tools.
	MemoryPressureWarn float64 `json:"memoryPressureWarn"`
	CoreMemoryTools    bool    `json:"coreMemoryTools"`

	// Context reset / handoff (Anthropic "harness design"). When HandoffAuto is on,
	// an autonomous turn that hits the context limit writes a handoff artifact and
	// continues in a FRESH session instead of only compacting in place.
	// HandoffPressure is the context-fill ratio that allows it; HandoffMaxChain caps
	// consecutive resets; HandoffWriteFile also drops the handoff to a file in the
	// working dir. 0 values select the built-in defaults.
	HandoffAuto      bool    `json:"handoffAuto"`
	HandoffPressure  float64 `json:"handoffPressure"`
	HandoffMaxChain  int     `json:"handoffMaxChain"`
	HandoffWriteFile bool    `json:"handoffWriteFile"`

	// Persistent progress (Anthropic claude-progress convention). When
	// ProgressPersist is on, the todo_write checklist is persisted to
	// <cwd>/.swarmgo/progress.json so it survives across sessions; ProgressResume
	// injects it back into a fresh session's context at start.
	ProgressPersist bool `json:"progressPersist"`
	ProgressResume  bool `json:"progressResume"`

	// FileFreshnessGuard (Claude Code parity). When on, the built-in Edit and Write
	// tools enforce a read-before-write / not-modified-since-read check: an edit (or
	// overwrite of an existing file) errors unless the file was read this session and
	// is unchanged since, so an out-of-band edit is never silently clobbered.
	FileFreshnessGuard bool `json:"fileFreshnessGuard"`

	// AutoTagSessions (event-driven auto-tagging). When on, the runtime derives
	// well-known session tags from turn outcomes + session state — "tool-error" (a
	// real tool failed; a claude-cli disallowed-tool denial is excluded), "error" (the
	// turn itself failed), "goal"/"goal-done"/"archived" — so an automation can scan
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

	// Auto-reflect (dream cycle): consolidate journals into a reflection once the
	// journal count crosses AutoReflectThreshold.
	AutoReflect          bool `json:"autoReflect"`
	AutoReflectThreshold int  `json:"autoReflectThreshold"`
	// AutoUserModel (HA-1): during the dream cycle, refresh the agent's "human"
	// core block from the journal so it learns durable facts about the user.
	AutoUserModel bool `json:"autoUserModel"`

	// Turn recovery (A1): structural handling of output-token cutoffs and context
	// overflow inside the native agentic tool loop.
	ReactiveCompact    bool `json:"reactiveCompact"`    // fold older history + retry on context overflow
	MaxTokenRetries    int  `json:"maxTokenRetries"`    // resume attempts after the output cap (0 = disabled)
	ReactiveKeepRecent int  `json:"reactiveKeepRecent"` // in-flight messages kept verbatim when compacting
	// MaxOutputTokens is the generation cap (max output tokens) applied when a
	// turn leaves it unset. 0 = auto: resolve per model family (providers.
	// MaxOutputFor), which keeps answers from being truncated at the providers'
	// 4096 fallback. A positive value pins a fixed global cap across all models
	// (overriding the family table); the SWARMGO_MAX_OUTPUT_TOKENS env is the
	// fallback when this is 0.
	MaxOutputTokens int `json:"maxOutputTokens"`

	// Tool-output token optimization — two independent, parallel systems applied
	// to tool results before they re-enter the model context.
	// System A: deterministic, free, rule-based (dedupe/group/truncate).
	CompactToolOutput bool `json:"compactToolOutput"` // System A master switch
	CompactMaxLines   int  `json:"compactMaxLines"`   // lines kept before middle elision (0 = default)
	CompactMaxBytes   int  `json:"compactMaxBytes"`   // hard byte cap after line work (0 = default)
	// System B: LLM intent-aware summary (costs a cheap model call, size-gated).
	CompactLLMSummary   bool   `json:"compactLlmSummary"`   // System B master switch
	CompactLLMThreshold int    `json:"compactLlmThreshold"` // only summarize output larger than this (bytes, 0 = default)
	CompactModel        string `json:"compactModel"`        // model id for System B; "" → TitleModel, then agent's own model

	// Auto-title generation.
	AutoTitleEnabled bool   `json:"autoTitleEnabled"`
	TitleModel       string `json:"titleModel"` // "" = use the agent's model

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
	// message. Supersedes --resume when on. Default off (experimental). _Docs/17.
	ClaudePersistentSession bool `json:"claudePersistentSession"`
	// run_subagent (isolated subagents / agent→agent delegation) is always installed;
	// availability is managed per-tool from the Tools screen. These remain as
	// per-turn safety guards on every delegation call.
	DelegationMaxDepth int `json:"delegationMaxDepth"` // max subagent nesting (0 = default 3)
	DelegationMaxCalls int `json:"delegationMaxCalls"` // max subagent runs per turn (0 = default 8)

	// Spawn guards — the detached background surface: run_subagent wait:"async"
	// (native) and the bridged spawn_session (claude-cli) + the UI spawn button.
	SpawnMaxConcurrent int `json:"spawnMaxConcurrent"` // max concurrent spawned sessions (0 = default 16)
	SpawnMaxPerTurn    int `json:"spawnMaxPerTurn"`    // max spawns per agent turn (0 = default 4)

	// Coordinator/worker guards (M2, _Docs/47).
	CoordinatorMaxWorkers int `json:"coordinatorMaxWorkers"` // max active workers per coordinator (0 = default 8)
	CoordinatorMaxTurns   int `json:"coordinatorMaxTurns"`   // max auto-triggered coordinator turns per session (0 = default 50)

	// Working-directory guards. The built-in fs/shell tools are unconfined (may
	// touch any path); these brake that power on autonomous (no-human) turns.
	AutonomousConfine    bool `json:"autonomousConfine"`    // confine fs/shell to the working dir on autonomous turns (default true)
	GitWorktreeIsolation bool `json:"gitWorktreeIsolation"` // give autonomous sessions a per-session git worktree (default false)
	AutonomousBootSeq    bool `json:"autonomousBootSeq"`    // inject the boot/verification-sequence reminder on autonomous turns (default true)

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
		Language:    "tr",

		DefaultProvider:       "claude-cli",
		DefaultModel:          "",
		DefaultPermissionMode: "auto",
		ClaudeCLIPath:         "",
		ClaudeConfigDir:       defaultClaudeConfigDir(),
		ClaudeCliAuthKind:     "",

		MaxContextTokens: 12000,
		KeepRecentMsgs:   8,
		RecallTopN:       5,
		// Recall cosine floor. 0.04 matches the historically hard-coded
		// memory.DefaultMinScore (the value actually in effect before this knob was
		// wired), so the default preserves existing recall behaviour. Raise it to
		// cut low-relevance recall noise from the dynamic context.
		RecallMinScore: 0.04,
		// Context-rot-aware default (2026-06-25, _Docs/17 §12): ceil 256K keeps the
		// live window in the gradient's high-precision zone; fraction 0 = "auto"
		// (per-family adaptive). Durability of folded detail comes from retrieval
		// (memory / conversation_search / core blocks), not from a huge raw window.
		ContextBudgetCeil:     262144,
		ContextBudgetFraction: 0,

		JournalCap:    50,
		JournalMaxLen: 1024,
		// Drop trivial per-turn journals (e.g. "Q: 2+2? A: 4") shorter than 40
		// runes so they never pollute recall. Conservative on purpose — a short but
		// substantive note survives; raise it to filter more aggressively, set 0 to
		// disable the gate entirely.
		JournalMinLen: 40,
		ReflectionCap: 20,

		// MemGPT memory: warn at 70% context fill, offer the core editing tools. Lowered
		// 0.75→0.70 (2026-06-25) to bring the "persist now" window earlier — pairs with
		// the smaller raw budget so important facts are written before the earlier fold.
		MemoryPressureWarn: 0.70,
		CoreMemoryTools:    true,

		// Context reset / handoff: off by default; the manual /handoff command and the
		// handoff_session tool work regardless. Defaults match agent.DefaultHandoff*.
		HandoffAuto:      false,
		HandoffPressure:  0.90,
		HandoffMaxChain:  20,
		HandoffWriteFile: false,

		// Persistent progress: on by default — only adds a per-project file and is
		// transparent to existing behaviour.
		ProgressPersist: true,
		ProgressResume:  true,

		// File freshness guard on by default (Claude Code parity): Edit/Write refuse to
		// clobber a file changed out-of-band since it was last read.
		FileFreshnessGuard: true,

		// Auto-tagging on by default: derives error/goal/archived tags for automation
		// scanning; add-only and cheap (a small write only when a tag actually changes).
		AutoTagSessions: true,

		// Debug journal on by default: only adds a per-session file, transparent to
		// existing behaviour. 5000 newest events kept per session.
		DebugJournalEnabled: true,
		DebugJournalCap:     5000,

		AutoReflect:          true,
		AutoReflectThreshold: 20,
		AutoUserModel:        true,

		ReactiveCompact:    true,
		MaxTokenRetries:    3,
		ReactiveKeepRecent: 6,
		MaxOutputTokens:    0, // auto: per-model family default

		// Both systems on by default, the external agent project-style: System A (free, deterministic)
		// always runs; System B (cheap-model summary) kicks in for big outputs. A's
		// byte cap (16KB) sits ABOVE B's threshold (12KB) so A's middle-elision never
		// pre-empts B's intelligent summary — outputs in the 12–16KB band reach B,
		// and anything larger is A-truncated to 16KB then B-summarized. Set a cheap
		// CompactModel (e.g. claude-haiku) so B stays inexpensive.
		CompactToolOutput:   true,
		CompactMaxLines:     200,
		CompactMaxBytes:     16384,
		CompactLLMSummary:   true,
		CompactLLMThreshold: 12288,
		CompactModel:        "",

		AutoTitleEnabled: true,
		TitleModel:       "",

		// CLI-path hooks default ON (preserves the hook-passthrough behaviour); turn
		// off when a hook authored for SwarmGo's shell misbehaves under the CLI's.
		EnableCLIHooks: true,

		// claude-cli session resume default ON: each turn passes --resume <id> and
		// sends only the delta, so the CLI reuses its warm server-side prompt cache
		// (cache_read instead of a full cold cache write). Combined with the static/
		// dynamic prompt split (see providers.ClaudeCLI.buildSystemAndPrompt), this
		// keeps the cached prefix warm turn-to-turn. _Docs/17.
		ClaudeResume: true,

		DelegationMaxDepth: 3,
		DelegationMaxCalls: 8,

		SpawnMaxConcurrent: 16,
		SpawnMaxPerTurn:    4,

		CoordinatorMaxWorkers: 8,
		CoordinatorMaxTurns:   50,

		// Autonomous turns confine fs/shell by default (safety brake); worktree
		// isolation is opt-in (needs git + has setup cost).
		AutonomousConfine:    true,
		GitWorktreeIsolation: false,
		AutonomousBootSeq:    true,

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

	DefaultProvider       string `json:"defaultProvider"`
	DefaultModel          string `json:"defaultModel"`
	DefaultPermissionMode string `json:"defaultPermissionMode"`
	ClaudeCLIPath         string `json:"claudeCliPath"`
	ClaudeConfigDir       string `json:"claudeConfigDir"`
	ClaudeCliAuthKind     string `json:"claudeCliAuthKind"`
	ClaudeCliAuthSet      bool   `json:"claudeCliAuthSet"`
	AnthropicKeySet       bool   `json:"anthropicKeySet"`
	MinimaxKeySet         bool   `json:"minimaxKeySet"`
	MinimaxBaseURL        string `json:"minimaxBaseUrl"`
	OpenRouterKeySet      bool   `json:"openrouterKeySet"`
	OpenRouterBaseURL     string `json:"openrouterBaseUrl"`

	CustomProviders []CustomProviderDTO `json:"customProviders"`

	ExtendedPromptCache bool `json:"extendedPromptCache"`

	DesktopNotifications bool `json:"desktopNotifications"`
	KeepAwake            bool `json:"keepAwake"`

	UserName     string `json:"userName"`
	UserTimezone string `json:"userTimezone"`
	UserCity     string `json:"userCity"`
	UserCountry  string `json:"userCountry"`
	UserNotes    string `json:"userNotes"`

	MaxContextTokens int     `json:"maxContextTokens"`
	KeepRecentMsgs   int     `json:"keepRecentMsgs"`
	RecallTopN       int     `json:"recallTopN"`
	RecallMinScore   float64 `json:"recallMinScore"`

	ContextBudgetCeil     int     `json:"contextBudgetCeil"`
	ContextBudgetFraction float64 `json:"contextBudgetFraction"`

	JournalCap    int `json:"journalCap"`
	JournalMaxLen int `json:"journalMaxLen"`
	JournalMinLen int `json:"journalMinLen"`
	ReflectionCap int `json:"reflectionCap"`

	MemoryPressureWarn float64 `json:"memoryPressureWarn"`
	CoreMemoryTools    bool    `json:"coreMemoryTools"`

	HandoffAuto      bool    `json:"handoffAuto"`
	HandoffPressure  float64 `json:"handoffPressure"`
	HandoffMaxChain  int     `json:"handoffMaxChain"`
	HandoffWriteFile bool    `json:"handoffWriteFile"`

	ProgressPersist bool `json:"progressPersist"`
	ProgressResume  bool `json:"progressResume"`

	FileFreshnessGuard bool `json:"fileFreshnessGuard"`
	AutoTagSessions    bool `json:"autoTagSessions"`

	DebugJournalEnabled bool `json:"debugJournalEnabled"`
	DebugJournalCap     int  `json:"debugJournalCap"`

	AutoReflect          bool `json:"autoReflect"`
	AutoReflectThreshold int  `json:"autoReflectThreshold"`
	AutoUserModel        bool `json:"autoUserModel"`

	ReactiveCompact    bool `json:"reactiveCompact"`
	MaxTokenRetries    int  `json:"maxTokenRetries"`
	ReactiveKeepRecent int  `json:"reactiveKeepRecent"`
	MaxOutputTokens    int  `json:"maxOutputTokens"`

	CompactToolOutput   bool   `json:"compactToolOutput"`
	CompactMaxLines     int    `json:"compactMaxLines"`
	CompactMaxBytes     int    `json:"compactMaxBytes"`
	CompactLLMSummary   bool   `json:"compactLlmSummary"`
	CompactLLMThreshold int    `json:"compactLlmThreshold"`
	CompactModel        string `json:"compactModel"`

	AutoTitleEnabled bool   `json:"autoTitleEnabled"`
	TitleModel       string `json:"titleModel"`

	EnableShell    bool `json:"enableShell"`
	EnableCLIHooks bool `json:"enableCliHooks"`
	EnableCodeMode bool `json:"enableCodeMode"`
	ClaudeResume   bool `json:"claudeResume"`
	// ClaudePersistentSession keeps ONE long-lived claude-cli process alive per
	// (session, agent) and feeds turns over stdin (stream-json input) instead of
	// spawning a fresh process each turn. The process holds the conversation
	// in-memory so warm turns ship only the new user message — maximal prompt-cache
	// reuse + no per-turn startup. Supersedes --resume when on. Default off
	// (experimental; validate live before enabling). _Docs/17.
	ClaudePersistentSession bool `json:"claudePersistentSession"`
	DelegationMaxDepth      int  `json:"delegationMaxDepth"`
	DelegationMaxCalls      int  `json:"delegationMaxCalls"`

	SpawnMaxConcurrent int `json:"spawnMaxConcurrent"`
	SpawnMaxPerTurn    int `json:"spawnMaxPerTurn"`

	CoordinatorMaxWorkers int `json:"coordinatorMaxWorkers"`
	CoordinatorMaxTurns   int `json:"coordinatorMaxTurns"`

	AutonomousConfine    bool `json:"autonomousConfine"`
	GitWorktreeIsolation bool `json:"gitWorktreeIsolation"`
	AutonomousBootSeq    bool `json:"autonomousBootSeq"`

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

		DefaultProvider:       s.DefaultProvider,
		DefaultModel:          s.DefaultModel,
		DefaultPermissionMode: s.DefaultPermissionMode,
		ClaudeCLIPath:         s.ClaudeCLIPath,
		ClaudeConfigDir:       s.ClaudeConfigDir,
		ClaudeCliAuthKind:     s.ClaudeCliAuthKind,
		ClaudeCliAuthSet:      s.ClaudeCliAuthTokenEnc != "",
		AnthropicKeySet:       s.AnthropicKeyEnc != "",
		MinimaxKeySet:         s.MinimaxKeyEnc != "",
		MinimaxBaseURL:        s.MinimaxBaseURL,
		OpenRouterKeySet:      s.OpenRouterKeyEnc != "",
		OpenRouterBaseURL:     s.OpenRouterBaseURL,
		CustomProviders:       customProvidersToDTO(s.CustomProviders),

		ExtendedPromptCache: s.ExtendedPromptCache,

		DesktopNotifications: s.DesktopNotifications,
		KeepAwake:            s.KeepAwake,

		UserName:     s.UserName,
		UserTimezone: s.UserTimezone,
		UserCity:     s.UserCity,
		UserCountry:  s.UserCountry,
		UserNotes:    s.UserNotes,

		MaxContextTokens: s.MaxContextTokens,
		KeepRecentMsgs:   s.KeepRecentMsgs,
		RecallTopN:       s.RecallTopN,
		RecallMinScore:   s.RecallMinScore,

		ContextBudgetCeil:     s.ContextBudgetCeil,
		ContextBudgetFraction: s.ContextBudgetFraction,

		JournalCap:    s.JournalCap,
		JournalMaxLen: s.JournalMaxLen,
		JournalMinLen: s.JournalMinLen,
		ReflectionCap: s.ReflectionCap,

		MemoryPressureWarn: s.MemoryPressureWarn,
		CoreMemoryTools:    s.CoreMemoryTools,

		HandoffAuto:      s.HandoffAuto,
		HandoffPressure:  s.HandoffPressure,
		HandoffMaxChain:  s.HandoffMaxChain,
		HandoffWriteFile: s.HandoffWriteFile,

		ProgressPersist: s.ProgressPersist,
		ProgressResume:  s.ProgressResume,

		FileFreshnessGuard: s.FileFreshnessGuard,
		AutoTagSessions:    s.AutoTagSessions,

		DebugJournalEnabled: s.DebugJournalEnabled,
		DebugJournalCap:     s.DebugJournalCap,

		AutoReflect:          s.AutoReflect,
		AutoReflectThreshold: s.AutoReflectThreshold,
		AutoUserModel:        s.AutoUserModel,

		ReactiveCompact:    s.ReactiveCompact,
		MaxTokenRetries:    s.MaxTokenRetries,
		ReactiveKeepRecent: s.ReactiveKeepRecent,
		MaxOutputTokens:    s.MaxOutputTokens,

		CompactToolOutput:   s.CompactToolOutput,
		CompactMaxLines:     s.CompactMaxLines,
		CompactMaxBytes:     s.CompactMaxBytes,
		CompactLLMSummary:   s.CompactLLMSummary,
		CompactLLMThreshold: s.CompactLLMThreshold,
		CompactModel:        s.CompactModel,

		AutoTitleEnabled: s.AutoTitleEnabled,
		TitleModel:       s.TitleModel,

		EnableShell:             s.EnableShell,
		EnableCLIHooks:          s.EnableCLIHooks,
		EnableCodeMode:          s.EnableCodeMode,
		ClaudeResume:            s.ClaudeResume,
		ClaudePersistentSession: s.ClaudePersistentSession,
		DelegationMaxDepth:      s.DelegationMaxDepth,
		DelegationMaxCalls:      s.DelegationMaxCalls,

		SpawnMaxConcurrent: s.SpawnMaxConcurrent,
		SpawnMaxPerTurn:    s.SpawnMaxPerTurn,

		CoordinatorMaxWorkers: s.CoordinatorMaxWorkers,
		CoordinatorMaxTurns:   s.CoordinatorMaxTurns,

		AutonomousConfine:    s.AutonomousConfine,
		GitWorktreeIsolation: s.GitWorktreeIsolation,
		AutonomousBootSeq:    s.AutonomousBootSeq,

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

	DefaultProvider       *string `json:"defaultProvider"`
	DefaultModel          *string `json:"defaultModel"`
	DefaultPermissionMode *string `json:"defaultPermissionMode"`
	ClaudeCLIPath         *string `json:"claudeCliPath"`
	ClaudeConfigDir       *string `json:"claudeConfigDir"`
	ClaudeCliAuthKind     *string `json:"claudeCliAuthKind"`
	ClaudeCliAuthToken    *string `json:"claudeCliAuthToken"` // write-only
	AnthropicKey          *string `json:"anthropicKey"`       // write-only
	MinimaxKey            *string `json:"minimaxKey"`         // write-only
	MinimaxBaseURL        *string `json:"minimaxBaseUrl"`
	OpenRouterKey         *string `json:"openrouterKey"` // write-only
	OpenRouterBaseURL     *string `json:"openrouterBaseUrl"`

	ExtendedPromptCache *bool `json:"extendedPromptCache"`

	DesktopNotifications *bool `json:"desktopNotifications"`
	KeepAwake            *bool `json:"keepAwake"`

	UserName     *string `json:"userName"`
	UserTimezone *string `json:"userTimezone"`
	UserCity     *string `json:"userCity"`
	UserCountry  *string `json:"userCountry"`
	UserNotes    *string `json:"userNotes"`

	MaxContextTokens *int     `json:"maxContextTokens"`
	KeepRecentMsgs   *int     `json:"keepRecentMsgs"`
	RecallTopN       *int     `json:"recallTopN"`
	RecallMinScore   *float64 `json:"recallMinScore"`

	ContextBudgetCeil     *int     `json:"contextBudgetCeil"`
	ContextBudgetFraction *float64 `json:"contextBudgetFraction"`

	JournalCap    *int `json:"journalCap"`
	JournalMaxLen *int `json:"journalMaxLen"`
	JournalMinLen *int `json:"journalMinLen"`
	ReflectionCap *int `json:"reflectionCap"`

	MemoryPressureWarn *float64 `json:"memoryPressureWarn"`
	CoreMemoryTools    *bool    `json:"coreMemoryTools"`

	HandoffAuto      *bool    `json:"handoffAuto"`
	HandoffPressure  *float64 `json:"handoffPressure"`
	HandoffMaxChain  *int     `json:"handoffMaxChain"`
	HandoffWriteFile *bool    `json:"handoffWriteFile"`

	ProgressPersist *bool `json:"progressPersist"`
	ProgressResume  *bool `json:"progressResume"`

	FileFreshnessGuard *bool `json:"fileFreshnessGuard"`
	AutoTagSessions    *bool `json:"autoTagSessions"`

	DebugJournalEnabled *bool `json:"debugJournalEnabled"`
	DebugJournalCap     *int  `json:"debugJournalCap"`

	AutoReflect          *bool `json:"autoReflect"`
	AutoReflectThreshold *int  `json:"autoReflectThreshold"`
	AutoUserModel        *bool `json:"autoUserModel"`

	ReactiveCompact    *bool `json:"reactiveCompact"`
	MaxTokenRetries    *int  `json:"maxTokenRetries"`
	ReactiveKeepRecent *int  `json:"reactiveKeepRecent"`
	MaxOutputTokens    *int  `json:"maxOutputTokens"`

	CompactToolOutput   *bool   `json:"compactToolOutput"`
	CompactMaxLines     *int    `json:"compactMaxLines"`
	CompactMaxBytes     *int    `json:"compactMaxBytes"`
	CompactLLMSummary   *bool   `json:"compactLlmSummary"`
	CompactLLMThreshold *int    `json:"compactLlmThreshold"`
	CompactModel        *string `json:"compactModel"`

	AutoTitleEnabled *bool   `json:"autoTitleEnabled"`
	TitleModel       *string `json:"titleModel"`

	EnableShell             *bool `json:"enableShell"`
	EnableCLIHooks          *bool `json:"enableCliHooks"`
	EnableCodeMode          *bool `json:"enableCodeMode"`
	ClaudeResume            *bool `json:"claudeResume"`
	ClaudePersistentSession *bool `json:"claudePersistentSession"`
	DelegationMaxDepth      *int  `json:"delegationMaxDepth"`
	DelegationMaxCalls      *int  `json:"delegationMaxCalls"`

	SpawnMaxConcurrent *int `json:"spawnMaxConcurrent"`
	SpawnMaxPerTurn    *int `json:"spawnMaxPerTurn"`

	CoordinatorMaxWorkers *int `json:"coordinatorMaxWorkers"`
	CoordinatorMaxTurns   *int `json:"coordinatorMaxTurns"`

	AutonomousConfine    *bool `json:"autonomousConfine"`
	GitWorktreeIsolation *bool `json:"gitWorktreeIsolation"`
	AutonomousBootSeq    *bool `json:"autonomousBootSeq"`

	BackupEnabled       *bool   `json:"backupEnabled"`
	BackupIntervalHours *int    `json:"backupIntervalHours"`
	BackupRetain        *int    `json:"backupRetain"`
	BackupDir           *string `json:"backupDir"`
}

// customProvidersToDTO masks the keys of a custom-provider list for the client.
func customProvidersToDTO(in []CustomProvider) []CustomProviderDTO {
	out := make([]CustomProviderDTO, 0, len(in))
	for _, c := range in {
		out = append(out, CustomProviderDTO{
			ID:           c.ID,
			Label:        c.Label,
			Kind:         c.Kind,
			BaseURL:      c.BaseURL,
			DefaultModel: c.DefaultModel,
			Models:       c.Models,
			KeySet:       c.KeyEnc != "",
			Reasoning:    c.Reasoning,
			PromptCache:  c.PromptCache,
		})
	}
	return out
}
