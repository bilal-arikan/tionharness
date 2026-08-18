// Package providers abstracts LLM endpoints behind a common interface so
// agents can switch between Anthropic, OpenAI, Ollama, etc.
package providers

import (
	"context"
	"encoding/json"
)

// Role identifies the author of a message.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// Stop reasons reported by Complete.
const (
	StopEndTurn = "end_turn" // model finished a normal textual reply
	StopToolUse = "tool_use" // model wants one or more tools executed
	StopMaxTok  = "max_tokens"
	// StopPauseTurn: a server-side tool loop (e.g. native tool search) hit its
	// internal iteration limit mid-turn. The caller resumes by re-sending the
	// conversation WITH the assistant's content appended verbatim (no extra user
	// message) — the server detects the trailing server-tool block and continues.
	StopPauseTurn = "pause_turn"
	// StopRefusal: safety classifiers declined the request (HTTP 200!). Content
	// is empty (pre-output, unbilled) or partial (mid-stream, billed — discard).
	// Fable-class models fire this most; see Response.StopDetails.
	StopRefusal = "refusal"
	// StopContextWindow: the model hit its CONTEXT WINDOW mid-turn (4.5+ signals
	// it as a stop reason, distinct from the max_tokens output cap). Recoverable
	// by compacting the in-flight history and retrying.
	StopContextWindow = "model_context_window_exceeded"
)

// StopDetails classifies a refusal (populated ONLY when StopReason ==
// StopRefusal): Category names the policy area ("cyber", "bio", … or ""),
// Explanation is an optional human-readable reason.
type StopDetails struct {
	Category    string `json:"category,omitempty"`
	Explanation string `json:"explanation,omitempty"`
}

// ToolDef describes a tool offered to the model. InputSchema is a JSON Schema
// object describing the tool's arguments.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	// Lazy marks a tool whose full schema is NOT shipped to the model up front:
	// only its name+description appear in a lightweight catalog, and the model
	// pulls the real schema on demand via activate_tools (progressive disclosure).
	// Not serialised to providers — it only informs catalog/request assembly.
	Lazy bool `json:"-"`
	// DeferLoading marks a tool for NATIVE (server-side) tool search: the full
	// def is included in the request but the server withholds it from the model's
	// context until discovered via the tool-search server tool — discovered
	// schemas are APPENDED, so the prompt-cache prefix survives. Providers
	// without native tool search ignore it (the def ships normally). The
	// Anthropic client auto-adds the search server tool when any def carries it.
	DeferLoading bool `json:"-"`
	// Strict requests API-side input validation (strict tool use): tool_use
	// inputs are GUARANTEED to validate against InputSchema. Requires the schema
	// to carry additionalProperties:false and a required array — the registry
	// normalizes both at request assembly. Ignored by providers without support;
	// dropped automatically on code-callable tools (incompatible with PTC).
	Strict bool `json:"-"`
	// CodeCallable marks the tool invocable from Claude-written code in the
	// code-execution container (programmatic tool calling): the anthropic client
	// maps it to allowed_callers:["code_execution_20260120"] when the request
	// enables PTC. Intermediate results stay out of the model's context.
	CodeCallable bool `json:"-"`
	// Examples are concrete sample tool calls (each a JSON object matching
	// InputSchema) that demonstrate usage conventions a schema alone cannot express
	// — date formats, ID patterns, which optional fields go together. They are
	// folded into the shipped InputSchema as a JSON Schema "examples" array at
	// request-assembly time (see registry foldExamples), so they travel only with
	// the FULL schema — never with the lightweight lazy catalog (name+desc only).
	// This is the TionSwarm analogue of the Anthropic "input_examples" tool field.
	Examples []json.RawMessage `json:"-"`
}

// ToolCall is a model request to run a tool. Input holds the raw JSON args.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	// Caller identifies how the call was made: "" / "direct" for ordinary tool
	// use, or a code-execution version ("code_execution_20260120") when Claude's
	// code invoked the tool programmatically. Programmatic batches constrain the
	// answering user message to PURE tool_result blocks (no text).
	Caller string `json:"caller,omitempty"`
}

// Programmatic reports whether this call came from code execution (PTC).
func (c ToolCall) Programmatic() bool {
	return c.Caller != "" && c.Caller != "direct"
}

// ToolResult is the outcome of executing a ToolCall, fed back to the model.
type ToolResult struct {
	CallID  string `json:"callId"`
	Content string `json:"content"`
	IsError bool   `json:"isError"`
}

// Message is a provider-agnostic chat turn. A turn may carry plain Text, and/or
// (for the assistant) ToolCalls it requested, and/or (for the user side) the
// ToolResults answering a previous assistant turn's tool calls.
type Message struct {
	Role        string
	Text        string
	ToolCalls   []ToolCall
	ToolResults []ToolResult
	// RawContent, when set, is the provider-native content-block array for this
	// turn, echoed back VERBATIM by providers that understand it (anthropic).
	// The native tool loop sets it on assistant turns from Response.RawContent so
	// server-side blocks the abstraction cannot model (tool search results,
	// server tool use) survive loop iterations exactly as the API requires.
	// Text/ToolCalls stay populated alongside as the portable fallback — every
	// other provider ignores RawContent and renders those instead.
	RawContent json.RawMessage
	// OnlyToolResults marks a user message answering a PROGRAMMATIC tool batch:
	// the API requires it to contain nothing but tool_result blocks, so the
	// anthropic client suppresses the volatile dynamic-suffix append (and any
	// other extra text) on it.
	OnlyToolResults bool
}

// Request is a completion request. Tools, when non-empty, enables tool use.
type Request struct {
	Model string
	// System is the STATIC system-prompt prefix: persona + user profile and other
	// content that is stable across calls for a given agent. With Anthropic prompt
	// caching, the cache breakpoint is placed at the end of this prefix (after the
	// tool definitions that precede it), so it is cached and reused turn-to-turn.
	System string
	// SystemDynamic is the VOLATILE system-prompt suffix appended after System:
	// recalled memory and anything that changes every turn. It is kept outside the
	// cached prefix so it never invalidates the cache. Providers without caching
	// simply concatenate it onto System.
	SystemDynamic string
	// Summary is the rolling compaction summary (already wrapped with its recovery
	// note) that stands in for the turns folded away by /compact. Unlike
	// SystemDynamic it is STABLE between two folds, so cache-capable providers place
	// it as a synthetic head message INSIDE the cached prefix (before the rolling
	// history breakpoint) — the Claude Code "compact boundary message" pattern — so
	// it becomes a cache READ turn-to-turn instead of being re-sent every turn.
	// Empty when the session has no summary. Providers without caching fold it back
	// into the system prompt (parity with the pre-P2 placement).
	Summary   string
	Messages  []Message
	MaxTokens int
	Tools     []ToolDef
	// ThinkingBudget, when > 0, requests extended reasoning with that many
	// thinking tokens (providers that support it, e.g. anthropic). 0 = off.
	ThinkingBudget int
	// TaskBudgetTokens, when > 0, tells the model how many tokens the WHOLE
	// agentic loop has (thinking + tool calls + output): the server injects a
	// running countdown the model sees and paces itself against — a soft,
	// model-aware complement to the caller's hard iteration caps. Only providers/
	// models with task-budget support send it (anthropic, adaptive class; see
	// SupportsTaskBudget); everyone else ignores it. Values below the API minimum
	// (20K) are raised to it.
	TaskBudgetTokens int
	// OutputSchema, when set, constrains the reply to this JSON Schema via
	// output_config.format (structured outputs). Only sent to models with
	// support (SupportsStructuredOutputs); other providers/models ignore it, so
	// callers MUST parse-with-fallback (the reply may be free text).
	OutputSchema json.RawMessage
	// ProgrammaticTools enables programmatic tool calling on providers with
	// support: the code-execution server tool is added and CodeCallable defs get
	// allowed_callers, letting Claude invoke tools from code with intermediate
	// results kept out of context. Ignored elsewhere.
	ProgrammaticTools bool
	// WebTools adds the server-side web search + web fetch tools (anthropic
	// only): searches run on Anthropic's infrastructure and return cited results
	// in the same response — no client-side execution. Billed per search, so the
	// client attaches conservative max_uses ceilings. Ignored elsewhere.
	WebTools bool
	// ContainerID resumes the code-execution container from a previous response
	// in the same turn (REQUIRED while a programmatic tool call is pending).
	ContainerID string
	// PermissionMode controls tool-use gating for providers that run their own
	// loop. The claude CLI maps it to its --permission-mode / --dangerously-skip-
	// permissions flags. "" | "auto" | "ask" | "read-only" (empty = auto).
	PermissionMode string
	// WorkDir, when set, is the working directory for providers that spawn a CLI
	// subprocess (claude-cli): its process cwd, so relative paths resolve inside
	// the workspace sandbox. Ignored by HTTP providers (anthropic/minimax).
	WorkDir string
	// OnEvent, when set, is called by providers that run the loop internally
	// (claude CLI) as each activity step (text/thinking/tool) becomes available,
	// enabling step-by-step streaming to the UI. Ignored by non-streaming
	// providers. Must be safe to call from the provider's goroutine.
	OnEvent func(TraceStep)
	// ResumeSessionID, when set, asks a CLI provider (claude-cli) to RESUME a prior
	// session (--resume <id>) instead of starting fresh: the CLI reuses its
	// server-side conversation + prompt cache, so the caller need only send the new
	// turn(s) rather than the full transcript (much cheaper). Empty = fresh session.
	// HTTP providers ignore it. See Response.SessionID for the (rotated) id to store
	// for the next turn.
	ResumeSessionID string
	// SysPromptFile, when true, tells a CLI provider (claude-cli) to hand the
	// appended system prompt through a temp file (--append-system-prompt-file <path>)
	// instead of inline (--append-system-prompt <text>, the default). File mode
	// sidesteps the Windows ~32 KB command-line limit for very large prompts; inline
	// leaves no temp file behind. HTTP providers ignore it.
	SysPromptFile bool
	// DisableThinking, when set, fully disables extended thinking on a CLI
	// provider's subprocess (claude-cli: MAX_THINKING_TOKENS=0 env). Set from the
	// agent's ThinkingLevel "Kapalı" so the CLI honours it instead of silently
	// falling back to its own adaptive-thinking default. Side benefit that
	// motivated it: claude-code ≥2.1.203 refuses PARALLEL tool calls while
	// thinking is active ("think XOR batch") — disabling thinking restores tool
	// batching and the pre-regression cost profile (_Docs/05 2026-07-10). HTTP
	// providers ignore it (ThinkingBudget==0 already means off there).
	DisableThinking bool
	// CLIEffortLevel is the resolved Claude Code effortLevel for a claude-cli turn
	// (low/medium/high/xhigh/max). Levels up to xhigh flow through the --settings
	// file; "max" is the exception — Claude Code's settings.json effortLevel enum
	// rejects it and silently downgrades to high, so the provider lifts a max turn
	// via the CLAUDE_CODE_EFFORT_LEVEL env var instead (the only channel the CLI
	// honours for max reasoning). Empty on non-max turns and for HTTP providers,
	// which ignore it.
	CLIEffortLevel string
}

// Usage reports token consumption. For providers with prompt caching, the cache
// counters are reported SEPARATELY from InputTokens (Anthropic's input_tokens
// already excludes cached tokens), so the true prompt size is InputTokens +
// CacheReadTokens + CacheWriteTokens. Cache reads are ~10× cheaper and cache
// writes a slight premium over fresh input — see providers.Price.
type Usage struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	CacheReadTokens  int `json:"cacheReadTokens,omitempty"`  // prompt tokens served from cache (cheap)
	CacheWriteTokens int `json:"cacheWriteTokens,omitempty"` // prompt tokens written to cache (premium)
	// ThinkingTokens is the ESTIMATED share of OutputTokens spent on hidden
	// extended reasoning. The API bills thinking inside OutputTokens without
	// breaking it out, so this is derived (output − visible) in the agent layer
	// (deriveThinkingTokens); providers leave it 0. Attribution/visibility only —
	// it is ALREADY part of OutputTokens, so billing must NOT add it again.
	ThinkingTokens int `json:"thinkingTokens,omitempty"`
}

// TraceStep is one entry in a provider-produced activity trace (intermediate
// text, thinking, or a tool call paired with its result). Providers that run
// the tool loop themselves (e.g. the claude CLI via stream-json) populate
// Response.Trace so the agent layer can surface the steps in the UI without
// driving the loop. Kept provider-local to avoid importing the agent package.
type TraceStep struct {
	Kind    string          // "text" | "thinking" | "tool"
	Text    string          // text/thinking payload
	Tool    string          // tool name
	Input   json.RawMessage // tool input
	Output  string          // tool result
	IsError bool            // tool failed
	// DurMs is the wall-clock latency of a "tool" step, measured by a provider that
	// runs the loop internally (claude-cli: time between seeing the tool_use event
	// and its tool_result on the live stream). 0 when unknown (native-loop steps,
	// which the agent layer times itself, or a non-streamed parse).
	DurMs int64
	// Batch groups "tool" steps that belong to ONE assistant message carrying
	// multiple parallel tool_use blocks (1-based id, unique within the turn;
	// 0 = lone call). Set by the claude-cli stream parser from message ids.
	Batch int
}

// Response is a completion result. When StopReason is StopToolUse, ToolCalls
// lists the tools the model wants executed before it continues.
type Response struct {
	Text       string
	ToolCalls  []ToolCall
	StopReason string
	Usage      Usage
	Model      string
	// Trace is an optional activity trace for providers that run the loop
	// internally (claude CLI). Empty for the native agentic loop, which the
	// agent layer traces itself.
	Trace []TraceStep
	// SessionID is the CLI provider's session id for this turn (claude-cli emits it
	// in the stream init/result events). The CLI rotates the id on each `-p --resume`
	// turn, so the caller must store THIS value to resume on the next turn. Empty for
	// providers without a resumable server-side session.
	SessionID string
	// ProviderCalls is the number of underlying model API round-trips this Response
	// aggregates. 0 for native single-call providers (one Complete == one call); for
	// claude-cli it is the CLI's internal tool-loop turn count (result event
	// num_turns), because the CLI reports Usage CUMULATIVELY across those steps — so
	// Usage divided by ProviderCalls recovers the per-call (single-pass) token cost.
	ProviderCalls int
	// RawContent is the provider-native content-block array of this response,
	// verbatim (anthropic only; empty elsewhere). The native tool loop echoes it
	// back on the assistant turn (Message.RawContent) so server-side blocks —
	// tool-search results, server tool use — survive loop iterations.
	RawContent json.RawMessage
	// ContainerID is the code-execution container of this response (PTC); pass
	// it back as Request.ContainerID on the next call of the same turn.
	ContainerID string
	// StopDetails carries the refusal classification when StopReason is
	// StopRefusal; nil for every other stop reason.
	StopDetails *StopDetails
}

// TokenCounter is implemented by providers exposing an exact server-side token
// count for a request (Anthropic /v1/messages/count_tokens). Used to display
// accurate figures next to the local heuristic estimate.
type TokenCounter interface {
	CountTokens(ctx context.Context, req Request) (int, error)
}

// Provider is implemented by every LLM backend.
type Provider interface {
	// Name returns the provider identifier, e.g. "anthropic".
	Name() string
	// Complete runs a non-streaming completion.
	Complete(ctx context.Context, req Request) (*Response, error)
}

// Delta kinds carried by StreamDelta.
const (
	DeltaText     = "text"     // visible answer text
	DeltaThinking = "thinking" // extended-reasoning (thinking) text
)

// StreamDelta is one incremental chunk emitted by a streaming provider. Kind
// routes the chunk in the UI: DeltaText feeds the live answer bubble while
// DeltaThinking feeds a live reasoning block. Text is never the empty string.
type StreamDelta struct {
	Kind string
	Text string
}

// Streamer is implemented by providers that can stream a completion's output
// token-by-token. Complete stays the baseline every provider must satisfy;
// Stream is a first-class optional capability the runtime prefers when a delta
// sink is available and the turn uses no tools. onDelta is called with each
// incremental chunk (never empty) from the provider's goroutine — text chunks
// and, when extended reasoning is on, thinking chunks (tagged via Kind). The
// returned Response carries the full accumulated answer text, usage and stop
// reason like Complete, plus an optional thinking TraceStep in Trace so the
// reasoning can be persisted.
type Streamer interface {
	Stream(ctx context.Context, req Request, onDelta func(StreamDelta)) (*Response, error)
}

// CanStream reports whether p supports incremental streaming (implements
// Streamer). Lets callers branch without a type assertion at each call site.
func CanStream(p Provider) bool {
	_, ok := p.(Streamer)
	return ok
}

// CLIMCPServer describes one MCP server for a CLI transport, in a shape both
// CLI dialects can render from: claude-cli writes an --mcp-config JSON file,
// codex-cli writes an [mcp_servers.<key>] block into its config.toml. Only the
// fields relevant to the chosen Transport are populated; the renderer on each
// side decides what it can express.
type CLIMCPServer struct {
	// Command and Args launch a stdio server; Env is added to its environment.
	Command string
	Args    []string
	Env     map[string]string
	// Transport selects the wire: "" or "stdio" for a spawned process, "sse" or
	// "http" for a remote endpoint described by URL/Headers.
	Transport string
	URL       string
	Headers   map[string]string
	// AlwaysLoad exempts the server from tool-search deferral on the claude
	// path (its tools are loaded up front). codex-cli has no such mechanism and
	// ignores this field.
	AlwaysLoad bool
}

// CLIMCPSpec is one turn's MCP delegation, transport-agnostic. It is the union
// of what the two CLI dialects need: the claude path consumes the pre-rendered
// ConfigPath plus its tool/permission/settings knobs, while a config-file
// dialect (codex) renders Servers itself. Fields a dialect cannot express are
// simply unused by it — see each field's comment.
type CLIMCPSpec struct {
	// ConfigPath is the already-written claude --mcp-config file.
	ConfigPath string
	// Servers is the structured server set, for dialects that render their own
	// config (codex). Unused on the claude path, which reads ConfigPath.
	Servers map[string]CLIMCPServer
	// AllowedTools / DisallowedTools / PermissionPrompt / SettingsPath are
	// claude-cli knobs (--allowedTools, --disallowedTools,
	// --permission-prompt-tool, --settings). codex-cli maps only a small subset.
	AllowedTools     []string
	DisallowedTools  []string
	PermissionPrompt string
	SettingsPath     string
}

// CLIProvider is implemented by providers that drive a locally-installed coding
// CLI as a subprocess and run the agentic tool loop inside it, instead of
// exposing a single-shot completion the native loop drives. The agent layer
// wires per-turn state (config home, MCP delegation) through this interface
// rather than asserting a concrete provider type, so adding a second CLI
// transport needs no branching in the agent layer. Steps that are genuinely
// specific to one CLI (claude-home credential healing, CLAUDE_CODE_EFFORT_LEVEL)
// stay behind a narrow concrete assertion at their call site.
type CLIProvider interface {
	Provider
	// SetConfigDir points the CLI at a config home for subsequent turns.
	SetConfigDir(dir string)
	// ConfigureCLIMCP installs one turn's MCP delegation.
	ConfigureCLIMCP(spec CLIMCPSpec)
	// Installed reports whether the CLI binary can be resolved. It says nothing
	// about login state.
	Installed() bool
}

// AuthProber is implemented by providers that can verify their credentials with
// a cheap, side-effect-free pre-flight call — for the CLI transports, a minimal
// tool-free invocation against the configured config home. It returns nil when
// authenticated and a classified error otherwise, so an auth lapse surfaces in
// Settings instead of failing the first real agent turn. Optional capability in
// the Streamer/TokenCounter mould: call sites branch on the interface, never on
// a concrete provider type.
type AuthProber interface {
	ProbeAuth(ctx context.Context) error
}

// Compile-time guard: the claude-cli transport is the capability's first
// implementer and the Settings pre-flight probe branches on the interface, so a
// signature drift here must fail the build rather than silently turn the probe
// into a "cannot probe authentication" error at runtime.
var _ AuthProber = (*ClaudeCLI)(nil)

// AsCLI returns p as a CLIProvider when it drives a coding CLI subprocess.
// Mirrors CanStream: it lets call sites branch on the capability without naming
// a concrete provider type.
func AsCLI(p Provider) (CLIProvider, bool) {
	c, ok := p.(CLIProvider)
	return c, ok
}
