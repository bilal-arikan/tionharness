// Package settings holds the application-global, user-editable configuration
// that backs the Settings screen. It is a single JSON document persisted in the
// data directory; the sensitive Anthropic API key is stored AES-GCM encrypted
// and never returned to clients in plaintext.
package settings

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
}

// Settings is the full, persisted configuration document. The encrypted
// Anthropic key lives in AnthropicKeyEnc and is never serialized to the API
// (json tag "-"); clients see only AnthropicKeySet via the DTO.
type Settings struct {
	// Appearance.
	Theme       string `json:"theme"`
	Accent      string `json:"accent"`      // hex color, e.g. "#4f8cff"
	ThemePreset string `json:"themePreset"` // curated palette id; "" = legacy theme+accent
	Language    string `json:"language"`    // "tr" | "en"

	// Providers.
	DefaultProvider string `json:"defaultProvider"` // "claude-cli" | "anthropic"
	DefaultModel    string `json:"defaultModel"`    // "" = provider default
	// DefaultPermissionMode seeds new agents' tool-use permission gate:
	// "read-only" | "ask" | "auto". "" falls back to "auto".
	DefaultPermissionMode string `json:"defaultPermissionMode"`
	ClaudeCLIPath         string `json:"claudeCliPath"`   // "" = auto-detect on PATH
	AnthropicKeyEnc       string `json:"anthropicKeyEnc"` // AES-GCM, never exposed

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
	OneMillionContext   bool `json:"oneMillionContext"`   // 1M-token context window beta
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

	// Journal (long-term memory) ring-buffer bounds.
	JournalCap    int `json:"journalCap"`    // newest journal entries kept per agent (0 = default)
	JournalMaxLen int `json:"journalMaxLen"` // max runes stored per journal entry (0 = default)

	// MemGPT-style self-editing memory (C6). MemoryPressureWarn is the context-fill
	// ratio (0..1) above which a turn warns the agent to persist important facts
	// before the next silent compaction; 0 disables the warning. CoreMemoryTools
	// offers the core_memory_replace/append editing tools.
	MemoryPressureWarn float64 `json:"memoryPressureWarn"`
	CoreMemoryTools    bool    `json:"coreMemoryTools"`

	// Auto-reflect (dream cycle): consolidate journals into a reflection once the
	// journal count crosses AutoReflectThreshold.
	AutoReflect          bool `json:"autoReflect"`
	AutoReflectThreshold int  `json:"autoReflectThreshold"`

	// Turn recovery (A1): structural handling of output-token cutoffs and context
	// overflow inside the native agentic tool loop.
	ReactiveCompact    bool `json:"reactiveCompact"`    // fold older history + retry on context overflow
	MaxTokenRetries    int  `json:"maxTokenRetries"`    // resume attempts after the output cap (0 = disabled)
	ReactiveKeepRecent int  `json:"reactiveKeepRecent"` // in-flight messages kept verbatim when compacting

	// Tool-output token optimization — two independent, parallel systems applied
	// to tool results before they re-enter the model context.
	// System A: deterministic, free, rule-based (dedupe/group/truncate).
	CompactToolOutput  bool `json:"compactToolOutput"`  // System A master switch
	CompactMaxLines    int  `json:"compactMaxLines"`    // lines kept before middle elision (0 = default)
	CompactMaxBytes    int  `json:"compactMaxBytes"`    // hard byte cap after line work (0 = default)
	// System B: LLM intent-aware summary (costs a cheap model call, size-gated).
	CompactLLMSummary   bool   `json:"compactLlmSummary"`   // System B master switch
	CompactLLMThreshold int    `json:"compactLlmThreshold"` // only summarize output larger than this (bytes, 0 = default)
	CompactModel        string `json:"compactModel"`        // model id for System B; "" → TitleModel, then agent's own model

	// Budget defaults applied to newly created agents (0 = unlimited).
	DefaultDailyCallLimit  int `json:"defaultDailyCallLimit"`
	DefaultDailyTokenLimit int `json:"defaultDailyTokenLimit"`

	// Autonomy.
	PauseAutonomy bool `json:"pauseAutonomy"`

	// Auto-title generation.
	AutoTitleEnabled bool   `json:"autoTitleEnabled"`
	TitleModel       string `json:"titleModel"` // "" = use the agent's model

	// MCP / tools.
	MCPGatewayURL string `json:"mcpGatewayUrl"`

	// Gated tool capabilities — off by default; each expands agent power/cost.
	EnableShell        bool `json:"enableShell"`        // built-in shell (arbitrary commands in sandbox)
	EnableSelfManage   bool `json:"enableSelfManage"`   // self-management suite (create/edit/delete entities)
	EnableCLIHooks     bool `json:"enableCliHooks"`     // pass PreToolUse/PostToolUse hooks to claude-cli agents via --settings
	EnableDelegation   bool `json:"enableDelegation"`   // run_subagent (isolated subagents / agent→agent delegation)
	DelegationMaxDepth int  `json:"delegationMaxDepth"` // max subagent nesting (0 = default 3)
	DelegationMaxCalls int  `json:"delegationMaxCalls"` // max subagent runs per turn (0 = default 8)

	// Spawn guards — the detached background surface: run_subagent wait:"async"
	// (native) and the bridged spawn_session (claude-cli) + the UI spawn button.
	SpawnMaxConcurrent int `json:"spawnMaxConcurrent"` // max concurrent spawned sessions (0 = default 16)
	SpawnMaxPerTurn    int `json:"spawnMaxPerTurn"`    // max spawns per agent turn (0 = default 4)

	// Working-directory guards. The built-in fs/shell tools are unconfined (may
	// touch any path); these brake that power on autonomous (no-human) turns.
	AutonomousConfine    bool `json:"autonomousConfine"`    // confine fs/shell to the working dir on autonomous turns (default true)
	GitWorktreeIsolation bool `json:"gitWorktreeIsolation"` // give autonomous sessions a per-session git worktree (default false)

	// Diagnostics (informational; applied on restart).
	LogLevel string `json:"logLevel"` // info | debug | warn | error
}

// Default returns the baseline settings used when no file exists yet. Values
// mirror the historical env/const defaults so behaviour is unchanged until the
// user edits anything.
func Default() Settings {
	return Settings{
		Theme:       ThemeDark,
		Accent:      "#8b5cf6",
		ThemePreset: "midnight-violet",
		Language:    "tr",

		DefaultProvider:       "claude-cli",
		DefaultModel:          "",
		DefaultPermissionMode: "auto",
		ClaudeCLIPath:         "",

		MaxContextTokens: 12000,
		KeepRecentMsgs:   8,
		RecallTopN:       5,
		RecallMinScore:   0.05,

		JournalCap:    50,
		JournalMaxLen: 1024,

		// MemGPT memory: warn at 75% context fill, offer the core editing tools.
		MemoryPressureWarn: 0.75,
		CoreMemoryTools:    true,

		AutoReflect:          true,
		AutoReflectThreshold: 30,

		ReactiveCompact:    true,
		MaxTokenRetries:    3,
		ReactiveKeepRecent: 6,

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

		DefaultDailyCallLimit:  0,
		DefaultDailyTokenLimit: 0,

		PauseAutonomy: false,

		AutoTitleEnabled: true,
		TitleModel:       "",

		MCPGatewayURL: "",

		// CLI-path hooks default ON (preserves the hook-passthrough behaviour); turn
		// off when a hook authored for SwarmGo's shell misbehaves under the CLI's.
		EnableCLIHooks: true,

		DelegationMaxDepth: 3,
		DelegationMaxCalls: 8,

		SpawnMaxConcurrent: 16,
		SpawnMaxPerTurn:    4,

		// Autonomous turns confine fs/shell by default (safety brake); worktree
		// isolation is opt-in (needs git + has setup cost).
		AutonomousConfine:    true,
		GitWorktreeIsolation: false,

		LogLevel: "info",
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
	AnthropicKeySet       bool   `json:"anthropicKeySet"`
	MinimaxKeySet         bool   `json:"minimaxKeySet"`
	MinimaxBaseURL        string `json:"minimaxBaseUrl"`
	OpenRouterKeySet      bool   `json:"openrouterKeySet"`
	OpenRouterBaseURL     string `json:"openrouterBaseUrl"`

	CustomProviders []CustomProviderDTO `json:"customProviders"`

	OneMillionContext   bool `json:"oneMillionContext"`
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

	JournalCap    int `json:"journalCap"`
	JournalMaxLen int `json:"journalMaxLen"`

	MemoryPressureWarn float64 `json:"memoryPressureWarn"`
	CoreMemoryTools    bool    `json:"coreMemoryTools"`

	AutoReflect          bool `json:"autoReflect"`
	AutoReflectThreshold int  `json:"autoReflectThreshold"`

	ReactiveCompact    bool `json:"reactiveCompact"`
	MaxTokenRetries    int  `json:"maxTokenRetries"`
	ReactiveKeepRecent int  `json:"reactiveKeepRecent"`

	CompactToolOutput   bool   `json:"compactToolOutput"`
	CompactMaxLines     int    `json:"compactMaxLines"`
	CompactMaxBytes     int    `json:"compactMaxBytes"`
	CompactLLMSummary   bool   `json:"compactLlmSummary"`
	CompactLLMThreshold int    `json:"compactLlmThreshold"`
	CompactModel        string `json:"compactModel"`

	DefaultDailyCallLimit  int `json:"defaultDailyCallLimit"`
	DefaultDailyTokenLimit int `json:"defaultDailyTokenLimit"`

	PauseAutonomy bool `json:"pauseAutonomy"`

	AutoTitleEnabled bool   `json:"autoTitleEnabled"`
	TitleModel       string `json:"titleModel"`

	MCPGatewayURL string `json:"mcpGatewayUrl"`

	EnableShell        bool `json:"enableShell"`
	EnableSelfManage   bool `json:"enableSelfManage"`
	EnableCLIHooks     bool `json:"enableCliHooks"`
	EnableDelegation   bool `json:"enableDelegation"`
	DelegationMaxDepth int  `json:"delegationMaxDepth"`
	DelegationMaxCalls int  `json:"delegationMaxCalls"`

	SpawnMaxConcurrent int `json:"spawnMaxConcurrent"`
	SpawnMaxPerTurn    int `json:"spawnMaxPerTurn"`

	AutonomousConfine    bool `json:"autonomousConfine"`
	GitWorktreeIsolation bool `json:"gitWorktreeIsolation"`

	LogLevel string `json:"logLevel"`
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
		AnthropicKeySet:       s.AnthropicKeyEnc != "",
		MinimaxKeySet:         s.MinimaxKeyEnc != "",
		MinimaxBaseURL:        s.MinimaxBaseURL,
		OpenRouterKeySet:      s.OpenRouterKeyEnc != "",
		OpenRouterBaseURL:     s.OpenRouterBaseURL,
		CustomProviders:       customProvidersToDTO(s.CustomProviders),

		OneMillionContext:   s.OneMillionContext,
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

		JournalCap:    s.JournalCap,
		JournalMaxLen: s.JournalMaxLen,

		MemoryPressureWarn: s.MemoryPressureWarn,
		CoreMemoryTools:    s.CoreMemoryTools,

		AutoReflect:          s.AutoReflect,
		AutoReflectThreshold: s.AutoReflectThreshold,

		ReactiveCompact:    s.ReactiveCompact,
		MaxTokenRetries:    s.MaxTokenRetries,
		ReactiveKeepRecent: s.ReactiveKeepRecent,

		CompactToolOutput:   s.CompactToolOutput,
		CompactMaxLines:     s.CompactMaxLines,
		CompactMaxBytes:     s.CompactMaxBytes,
		CompactLLMSummary:   s.CompactLLMSummary,
		CompactLLMThreshold: s.CompactLLMThreshold,
		CompactModel:        s.CompactModel,

		DefaultDailyCallLimit:  s.DefaultDailyCallLimit,
		DefaultDailyTokenLimit: s.DefaultDailyTokenLimit,

		PauseAutonomy: s.PauseAutonomy,

		AutoTitleEnabled: s.AutoTitleEnabled,
		TitleModel:       s.TitleModel,

		MCPGatewayURL: s.MCPGatewayURL,

		EnableShell:        s.EnableShell,
		EnableSelfManage:   s.EnableSelfManage,
		EnableCLIHooks:     s.EnableCLIHooks,
		EnableDelegation:   s.EnableDelegation,
		DelegationMaxDepth: s.DelegationMaxDepth,
		DelegationMaxCalls: s.DelegationMaxCalls,

		SpawnMaxConcurrent: s.SpawnMaxConcurrent,
		SpawnMaxPerTurn:    s.SpawnMaxPerTurn,

		AutonomousConfine:    s.AutonomousConfine,
		GitWorktreeIsolation: s.GitWorktreeIsolation,

		LogLevel: s.LogLevel,
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
	AnthropicKey          *string `json:"anthropicKey"` // write-only
	MinimaxKey            *string `json:"minimaxKey"`   // write-only
	MinimaxBaseURL        *string `json:"minimaxBaseUrl"`
	OpenRouterKey         *string `json:"openrouterKey"` // write-only
	OpenRouterBaseURL     *string `json:"openrouterBaseUrl"`

	OneMillionContext   *bool `json:"oneMillionContext"`
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

	JournalCap    *int `json:"journalCap"`
	JournalMaxLen *int `json:"journalMaxLen"`

	MemoryPressureWarn *float64 `json:"memoryPressureWarn"`
	CoreMemoryTools    *bool    `json:"coreMemoryTools"`

	AutoReflect          *bool `json:"autoReflect"`
	AutoReflectThreshold *int  `json:"autoReflectThreshold"`

	ReactiveCompact    *bool `json:"reactiveCompact"`
	MaxTokenRetries    *int  `json:"maxTokenRetries"`
	ReactiveKeepRecent *int  `json:"reactiveKeepRecent"`

	CompactToolOutput   *bool   `json:"compactToolOutput"`
	CompactMaxLines     *int    `json:"compactMaxLines"`
	CompactMaxBytes     *int    `json:"compactMaxBytes"`
	CompactLLMSummary   *bool   `json:"compactLlmSummary"`
	CompactLLMThreshold *int    `json:"compactLlmThreshold"`
	CompactModel        *string `json:"compactModel"`

	DefaultDailyCallLimit  *int `json:"defaultDailyCallLimit"`
	DefaultDailyTokenLimit *int `json:"defaultDailyTokenLimit"`

	PauseAutonomy *bool `json:"pauseAutonomy"`

	AutoTitleEnabled *bool   `json:"autoTitleEnabled"`
	TitleModel       *string `json:"titleModel"`

	MCPGatewayURL *string `json:"mcpGatewayUrl"`

	EnableShell        *bool `json:"enableShell"`
	EnableSelfManage   *bool `json:"enableSelfManage"`
	EnableCLIHooks     *bool `json:"enableCliHooks"`
	EnableDelegation   *bool `json:"enableDelegation"`
	DelegationMaxDepth *int  `json:"delegationMaxDepth"`
	DelegationMaxCalls *int  `json:"delegationMaxCalls"`

	SpawnMaxConcurrent *int `json:"spawnMaxConcurrent"`
	SpawnMaxPerTurn    *int `json:"spawnMaxPerTurn"`

	AutonomousConfine    *bool `json:"autonomousConfine"`
	GitWorktreeIsolation *bool `json:"gitWorktreeIsolation"`

	LogLevel *string `json:"logLevel"`
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
		})
	}
	return out
}
