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
	StopEndTurn = "end_turn"  // model finished a normal textual reply
	StopToolUse = "tool_use"  // model wants one or more tools executed
	StopMaxTok  = "max_tokens"
)

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
	// Examples are concrete sample tool calls (each a JSON object matching
	// InputSchema) that demonstrate usage conventions a schema alone cannot express
	// — date formats, ID patterns, which optional fields go together. They are
	// folded into the shipped InputSchema as a JSON Schema "examples" array at
	// request-assembly time (see registry foldExamples), so they travel only with
	// the FULL schema — never with the lightweight lazy catalog (name+desc only).
	// This is the SwarmGo analogue of the Anthropic "input_examples" tool field.
	Examples []json.RawMessage `json:"-"`
}

// ToolCall is a model request to run a tool. Input holds the raw JSON args.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
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
	// recalled memory, the running conversation summary, and anything that changes
	// every turn. It is kept outside the cached prefix so it never invalidates the
	// cache. Providers without caching simply concatenate it onto System.
	SystemDynamic string
	Messages      []Message
	MaxTokens     int
	Tools         []ToolDef
	// ThinkingBudget, when > 0, requests extended reasoning with that many
	// thinking tokens (providers that support it, e.g. anthropic). 0 = off.
	ThinkingBudget int
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
