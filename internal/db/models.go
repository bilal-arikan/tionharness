package db

// Agent is an autonomous AI entity bound to a provider/model.
type Agent struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Soul         string `json:"soul"`
	Identity     string `json:"identity"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	PlanningMode string `json:"planningMode"`
	// ThinkingLevel requests extended reasoning: "" / "off" | "low" | "medium" |
	// "high". Applied on plain (non-tool) completions; anthropic provider only.
	ThinkingLevel string `json:"thinkingLevel"`
	// PermissionMode gates how the agent's tool use is approved:
	// "read-only" | "ask" | "auto". Empty defaults to "auto". For the claude-cli
	// path this maps to the CLI's --permission-mode / --dangerously-skip-permissions
	// flags (headless mode refuses edits without an explicit mode).
	PermissionMode string `json:"permissionMode"`

	// Visual identity for the roster avatar. Avatar holds an optional emoji/glyph
	// rendered inside the circle; Color is an optional hex accent (e.g. "#7c3aed").
	// Both may be empty — the frontend then derives a deterministic look from ID.
	Avatar string `json:"avatar"`
	Color  string `json:"color"`

	// Daily spend caps for autonomous calls (0 = unlimited).
	DailyCallLimit  int `json:"dailyCallLimit"`
	DailyTokenLimit int `json:"dailyTokenLimit"`

	// Tool access. MCPEnabled gates whether the agent is offered tools at all.
	//
	// Agents reach EVERY workspace-active tool by default. Access is narrowed two
	// independent ways:
	//   - BlockedTools: per-agent denylist (the user-facing model in agent
	//     detail). Empty = nothing blocked = all tools. New tools added later are
	//     reachable automatically unless explicitly blocked here.
	//   - AllowedTools: a legacy allowlist of tool-name patterns. Retained because
	//     the built-in subagent profiles (explore/coder/reviewer) restrict an
	//     isolated worker to a fixed tool set. Empty = allow all. User-facing
	//     agents leave this empty and use the denylist instead.
	// Both compose: a tool is offered iff it is not blocked AND (the allowlist is
	// empty OR matches it).
	MCPEnabled   bool   `json:"mcpEnabled"`
	AllowedTools string `json:"allowedTools"` // JSON array (legacy allowlist; subagent profiles)
	BlockedTools string `json:"blockedTools"` // JSON array (per-agent denylist)

	// Skills is the list of skill slugs enabled for this agent. Only these skills
	// are advertised to the agent and loadable via use_skill. Empty means the
	// agent has no assigned skills. Skills themselves are a shared library
	// (global/workspace/project tiers) — never agent-owned.
	Skills []string `json:"skills"`

	// CoreBlocks defines this agent's named core-memory blocks (MemGPT memory
	// blocks): their labels, char limits, descriptions and read-only flags. The
	// block text itself lives in knowledge_sources under CoreKind(label). Empty
	// means the agent uses DefaultCoreBlocks (persona + human) — no migration for
	// agents created before named blocks existed.
	CoreBlocks []CoreBlock `json:"coreBlocks,omitempty"`

	// CreatedBy records the ID of the agent that created this agent through a
	// self-management tool. Empty means it was created by the user (UI/API).
	// Agents may only edit/delete entities that were created by an agent.
	CreatedBy string `json:"createdBy,omitempty"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// SessionSchemaVersion is the current session-header format version, stamped on
// new sessions (Session.SchemaVersion). Bump it whenever header fields are added
// so a future loader can branch on the version. 1 = first versioned header
// (added labels/status/pinned + the enriched per-message fields).
const SessionSchemaVersion = 1

// Session is a conversation thread belonging to an agent.
//
// Kind is the broad category of what produced the transcript:
//
//	chat      manual user conversation (default)
//	task      a kanban task's run history (one session per task)
//	flow      an orchestration flow's run history (one session per flow)
//	schedule  an agent's scheduled-prompt deliveries (one per agent)
//
// SourceID links the session back to the entity that owns it (a task or flow id);
// it is empty for plain chat and for agent-keyed kinds (schedule). This
// is the unification primitive: every execution path funnels its output into a
// Session, so a single streamable transcript viewer and the unified "executions"
// feed can render task runs, flow runs and scheduled deliveries like any chat.
type Session struct {
	ID           string `json:"id"`
	AgentID      string `json:"agentId"`
	Kind         string `json:"kind"`
	SourceID     string `json:"sourceId,omitempty"`
	Title        string `json:"title"`
	MessageCount int    `json:"messageCount"`
	State        string `json:"state"`
	// Unread is true when an agent reply landed in this session while it was not
	// the one being viewed; cleared when the user opens it.
	Unread bool `json:"unread"`

	// Goal is a persistent, user-set objective ("north star") for this session.
	// When set it is injected into every turn's context so the agent keeps its
	// replies aligned with it. Inspired by Claude Code's /goal: one durable,
	// measurable objective that steers the conversation. Empty = no goal.
	Goal string `json:"goal,omitempty"`
	// GoalDone marks the goal as achieved: it stays stored (so the user can
	// review or reopen it) but is no longer injected into context — completing a
	// goal stops it steering future turns.
	GoalDone bool `json:"goalDone,omitempty"`

	// WorkingDir is this session's working directory (cwd) for the built-in
	// filesystem/shell tools — like `cd /path/to/project` in a terminal. When set
	// it overrides the workspace default: relative paths resolve here and the
	// shell starts here, and the agent is told its cwd + git branch in context.
	// Empty = use the workspace default working dir. Inspired by the external agent project's
	// per-session working directory.
	WorkingDir string `json:"workingDir,omitempty"`

	// SchemaVersion is the session-header format version, bumped whenever new
	// header fields are added so a future loader can detect and migrate old headers.
	// 0 = pre-versioning (sessions created before this field). Set at creation.
	SchemaVersion int `json:"v,omitempty"`

	// Labels are free-form tags on the session for filtering and automation
	// (user- or agent-set). Distinct from State (lifecycle) and Status (workflow).
	Labels []string `json:"labels,omitempty"`
	// Status is a free-form WORKFLOW state for the session ("in_progress",
	// "blocked", "done", "review", …) — orthogonal to State, which is the lifecycle
	// (active/archived). Empty = none. Drives sidebar filters and status automations.
	Status string `json:"status,omitempty"`
	// Pinned keeps the session at the top of the sidebar list regardless of recency.
	Pinned bool `json:"pinned,omitempty"`

	// Conversation compaction state (see internal/conversation).
	Summary         string `json:"summary"`
	SummaryMsgCount int    `json:"summaryMsgCount"`

	// Context-reset / handoff lineage (see internal/agent/handoff.go). When a
	// session nears its context limit it is not just compacted in place: a handoff
	// artifact is written and a FRESH session is spawned to continue the work in a
	// clean window (Anthropic "context reset" pattern). ParentSessionID points back
	// to the session this one continues, so the UI can walk the reset chain;
	// HandoffArtifactID is the handoff artifact written into the PARENT at reset.
	// Both empty for an ordinary (non-handoff) session.
	ParentSessionID   string `json:"parentSessionId,omitempty"`
	HandoffArtifactID string `json:"handoffArtifactId,omitempty"`

	// claude-cli session resume (opt-in, ClaudeResume setting). CLISessionID is the
	// CLI's server-side session to --resume on the next turn (rotates each turn);
	// CLISentMsgCount is how many of this session's messages the CLI has already
	// seen, so the next turn sends only the newer ones (the delta). Empty/0 = no
	// warm CLI session yet (next turn starts cold and captures a fresh id).
	CLISessionID    string `json:"cliSessionId,omitempty"`
	CLISentMsgCount int    `json:"cliSentMsgCount,omitempty"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// Message is a single turn within a session.
type Message struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Role      string `json:"role"` // system | user | assistant | tool
	// AgentID records which agent produced an assistant turn (empty for user/
	// system). In a multi-agent session different turns may come from different
	// agents (via "@mention" routing); the UI shows each turn's agent.
	AgentID          string `json:"agentId"`
	Text             string `json:"text"`
	ToolCalls        string `json:"toolCalls"`
	ReasoningContent string `json:"reasoningContent"`
	// Steps is a JSON array of agent.TurnStep records: the ordered trace of
	// thinking, intermediate text and tool calls behind this turn. Empty for
	// plain (non-tool) replies. Drives the rich chat turn renderer.
	Steps string `json:"steps"`
	// Interrupted marks an assistant reply that was reconstructed from a crash
	// sidecar (the process died mid-stream): the text/trace are partial and the UI
	// flags the turn as cut off. Empty/false for normal turns.
	Interrupted bool `json:"interrupted,omitempty"`
	// Cancelled marks an assistant reply the USER stopped mid-turn (Shift+stop) —
	// distinct from Interrupted (a crash-recovered partial). The text/trace are
	// whatever completed before the stop. Empty/false for normal turns.
	Cancelled bool `json:"cancelled,omitempty"`
	// Model is the provider model that actually served THIS assistant turn (the
	// agent's model can change, and a turn may carry a per-turn override). Empty for
	// user/system turns.
	Model string `json:"model,omitempty"`
	// StopReason is why generation ended: "end_turn" | "max_tokens" | "tool_use" |
	// "refusal" | "stop_sequence". Lets the UI flag a truncated/refused reply
	// without re-deriving it. Empty for user/system or unknown.
	StopReason string `json:"stopReason,omitempty"`
	// Usage is THIS turn's token consumption (the per-bubble cost), so the
	// transcript is self-contained for export/analysis without cross-referencing
	// the usage rollup or debug journal. Nil for user/system or non-LLM turns.
	Usage *MessageUsage `json:"usage,omitempty"`
	// DurationMs is the wall-clock time this assistant turn took (request→reply).
	// 0 when unknown (user/system, or not measured).
	DurationMs int64 `json:"durationMs,omitempty"`
	// Feedback is the user's rating of an assistant turn (👍/👎 + optional note) —
	// a durable quality signal the reflector/eval can learn from. Nil = no rating.
	Feedback *MessageFeedback `json:"feedback,omitempty"`
	// Origin marks how a prompt-bearing message was produced, for display only:
	// "wake" = a schedule_wake auto-resume, "schedule" = a scheduled routine prompt.
	// Empty = a real user/assistant message. The role stays "user" so the model's
	// replayed context is unchanged; the UI keys off this to render auto prompts as
	// a "⏰ continuation" note instead of a user bubble (so the agent doesn't look
	// like it is asking itself).
	Origin string `json:"origin,omitempty"`
	// Attachments are user-supplied files (or pasted long text) sent with this
	// message. Stored on user turns; empty for assistant/system. The files live
	// under the workspace's uploads/ directory so agents can read them via their
	// sandboxed read_file tool (see Attachment.RelPath).
	Attachments []Attachment `json:"attachments,omitempty"`
	CreatedAt   int64        `json:"createdAt"`
}

// MessageUsage is a single assistant turn's token consumption, stored on the
// message so the transcript carries its own per-turn cost (compact keys to keep
// the JSONL line small). Mirrors providers.Usage but kept dependency-free here.
type MessageUsage struct {
	InputTokens      int `json:"in"`
	OutputTokens     int `json:"out"`
	CacheReadTokens  int `json:"cacheRead,omitempty"`
	CacheWriteTokens int `json:"cacheWrite,omitempty"`
}

// MessageFeedback is the user's rating of an assistant turn. Rating is +1 (up),
// -1 (down) or 0 (cleared); Note is an optional free-text comment; At is the
// unix-second timestamp the rating was set.
type MessageFeedback struct {
	Rating int    `json:"rating"`
	Note   string `json:"note,omitempty"`
	At     int64  `json:"at"`
}
