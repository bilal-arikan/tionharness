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

	// Auto-reflect (dream cycle): consolidate journals into a reflection once the
	// journal count crosses AutoReflectThreshold.
	AutoReflect          bool `json:"autoReflect"`
	AutoReflectThreshold int  `json:"autoReflectThreshold"`

	// Turn recovery (A1): structural handling of output-token cutoffs and context
	// overflow inside the native agentic tool loop.
	ReactiveCompact    bool `json:"reactiveCompact"`    // fold older history + retry on context overflow
	MaxTokenRetries    int  `json:"maxTokenRetries"`    // resume attempts after the output cap (0 = disabled)
	ReactiveKeepRecent int  `json:"reactiveKeepRecent"` // in-flight messages kept verbatim when compacting

	// Budget defaults applied to newly created agents (0 = unlimited).
	DefaultDailyCallLimit  int `json:"defaultDailyCallLimit"`
	DefaultDailyTokenLimit int `json:"defaultDailyTokenLimit"`

	// Autonomy.
	DefaultHeartbeatSec int  `json:"defaultHeartbeatSec"`
	PauseAutonomy       bool `json:"pauseAutonomy"`

	// Auto-title generation.
	AutoTitleEnabled bool   `json:"autoTitleEnabled"`
	TitleModel       string `json:"titleModel"` // "" = use the agent's model

	// MCP / tools.
	MCPGatewayURL string `json:"mcpGatewayUrl"`

	// Gated tool capabilities — off by default; each expands agent power/cost.
	EnableShell        bool `json:"enableShell"`        // built-in shell (arbitrary commands in sandbox)
	EnableSelfManage   bool `json:"enableSelfManage"`   // self-management suite (create/edit/delete entities)
	EnableDelegation   bool `json:"enableDelegation"`   // call_agent (agent→agent delegation)
	DelegationMaxDepth int  `json:"delegationMaxDepth"` // max delegation nesting (0 = default 3)
	DelegationMaxCalls int  `json:"delegationMaxCalls"` // max delegations per turn (0 = default 8)

	// Cross-session awareness: inject a short summary of the workspace's active +
	// recent chat sessions into an agent's dynamic context so it starts aware of
	// the rest of the work. Also gates the list_sessions pull tool.
	SessionContextEnabled     bool `json:"sessionContextEnabled"`     // master toggle (push block + pull tool)
	SessionContextEveryTurn   bool `json:"sessionContextEveryTurn"`   // false = inject only on a session's first turn
	SessionContextRecentCount int  `json:"sessionContextRecentCount"` // # of past (non-active) sessions to list (0 = default 5)

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

		AutoReflect:          true,
		AutoReflectThreshold: 30,

		ReactiveCompact:    true,
		MaxTokenRetries:    3,
		ReactiveKeepRecent: 6,

		DefaultDailyCallLimit:  0,
		DefaultDailyTokenLimit: 0,

		DefaultHeartbeatSec: 60,
		PauseAutonomy:       false,

		AutoTitleEnabled: true,
		TitleModel:       "",

		MCPGatewayURL: "",

		DelegationMaxDepth: 3,
		DelegationMaxCalls: 8,

		SessionContextEnabled:     true,
		SessionContextEveryTurn:   false,
		SessionContextRecentCount: 5,

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

	AutoReflect          bool `json:"autoReflect"`
	AutoReflectThreshold int  `json:"autoReflectThreshold"`

	ReactiveCompact    bool `json:"reactiveCompact"`
	MaxTokenRetries    int  `json:"maxTokenRetries"`
	ReactiveKeepRecent int  `json:"reactiveKeepRecent"`

	DefaultDailyCallLimit  int `json:"defaultDailyCallLimit"`
	DefaultDailyTokenLimit int `json:"defaultDailyTokenLimit"`

	DefaultHeartbeatSec int  `json:"defaultHeartbeatSec"`
	PauseAutonomy       bool `json:"pauseAutonomy"`

	AutoTitleEnabled bool   `json:"autoTitleEnabled"`
	TitleModel       string `json:"titleModel"`

	MCPGatewayURL string `json:"mcpGatewayUrl"`

	EnableShell        bool `json:"enableShell"`
	EnableSelfManage   bool `json:"enableSelfManage"`
	EnableDelegation   bool `json:"enableDelegation"`
	DelegationMaxDepth int  `json:"delegationMaxDepth"`
	DelegationMaxCalls int  `json:"delegationMaxCalls"`

	SessionContextEnabled     bool `json:"sessionContextEnabled"`
	SessionContextEveryTurn   bool `json:"sessionContextEveryTurn"`
	SessionContextRecentCount int  `json:"sessionContextRecentCount"`

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

		AutoReflect:          s.AutoReflect,
		AutoReflectThreshold: s.AutoReflectThreshold,

		ReactiveCompact:    s.ReactiveCompact,
		MaxTokenRetries:    s.MaxTokenRetries,
		ReactiveKeepRecent: s.ReactiveKeepRecent,

		DefaultDailyCallLimit:  s.DefaultDailyCallLimit,
		DefaultDailyTokenLimit: s.DefaultDailyTokenLimit,

		DefaultHeartbeatSec: s.DefaultHeartbeatSec,
		PauseAutonomy:       s.PauseAutonomy,

		AutoTitleEnabled: s.AutoTitleEnabled,
		TitleModel:       s.TitleModel,

		MCPGatewayURL: s.MCPGatewayURL,

		EnableShell:        s.EnableShell,
		EnableSelfManage:   s.EnableSelfManage,
		EnableDelegation:   s.EnableDelegation,
		DelegationMaxDepth: s.DelegationMaxDepth,
		DelegationMaxCalls: s.DelegationMaxCalls,

		SessionContextEnabled:     s.SessionContextEnabled,
		SessionContextEveryTurn:   s.SessionContextEveryTurn,
		SessionContextRecentCount: s.SessionContextRecentCount,

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

	AutoReflect          *bool `json:"autoReflect"`
	AutoReflectThreshold *int  `json:"autoReflectThreshold"`

	ReactiveCompact    *bool `json:"reactiveCompact"`
	MaxTokenRetries    *int  `json:"maxTokenRetries"`
	ReactiveKeepRecent *int  `json:"reactiveKeepRecent"`

	DefaultDailyCallLimit  *int `json:"defaultDailyCallLimit"`
	DefaultDailyTokenLimit *int `json:"defaultDailyTokenLimit"`

	DefaultHeartbeatSec *int  `json:"defaultHeartbeatSec"`
	PauseAutonomy       *bool `json:"pauseAutonomy"`

	AutoTitleEnabled *bool   `json:"autoTitleEnabled"`
	TitleModel       *string `json:"titleModel"`

	MCPGatewayURL *string `json:"mcpGatewayUrl"`

	EnableShell        *bool `json:"enableShell"`
	EnableSelfManage   *bool `json:"enableSelfManage"`
	EnableDelegation   *bool `json:"enableDelegation"`
	DelegationMaxDepth *int  `json:"delegationMaxDepth"`
	DelegationMaxCalls *int  `json:"delegationMaxCalls"`

	SessionContextEnabled     *bool `json:"sessionContextEnabled"`
	SessionContextEveryTurn   *bool `json:"sessionContextEveryTurn"`
	SessionContextRecentCount *int  `json:"sessionContextRecentCount"`

	LogLevel *string `json:"logLevel"`
}
