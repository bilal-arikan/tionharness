package db

// Agent is an autonomous AI entity bound to a provider/model.
type Agent struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Soul         string `json:"soul"`
	Identity     string `json:"identity"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Capabilities string `json:"capabilities"` // JSON array
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

	// Heartbeat (autonomous wake) configuration.
	HeartbeatEnabled     bool   `json:"heartbeatEnabled"`
	HeartbeatIntervalSec int    `json:"heartbeatIntervalSec"`
	HeartbeatPrompt      string `json:"heartbeatPrompt"`

	// Daily spend caps for autonomous calls (0 = unlimited).
	DailyCallLimit  int `json:"dailyCallLimit"`
	DailyTokenLimit int `json:"dailyTokenLimit"`

	// Tool access. MCPEnabled gates whether the agent is offered tools at all;
	// AllowedTools is an optional JSON allowlist of tool-name patterns.
	MCPEnabled   bool   `json:"mcpEnabled"`
	AllowedTools string `json:"allowedTools"` // JSON array

	// Skills is the ordered list of skill slugs enabled for this agent. Only
	// these skills are advertised to the agent (in this order) and loadable via
	// use_skill. Empty means the agent has no skills. Skills themselves are a
	// shared library (global/workspace/project tiers) — never agent-owned.
	Skills []string `json:"skills"`

	// CreatedBy records the ID of the agent that created this agent through a
	// self-management tool. Empty means it was created by the user (UI/API).
	// Agents may only edit/delete entities that were created by an agent.
	CreatedBy string `json:"createdBy,omitempty"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// Session is a conversation thread belonging to an agent.
//
// Kind is the broad category of what produced the transcript:
//
//	chat      manual user conversation (default)
//	task      a kanban task's run history (one session per task)
//	flow      an orchestration flow's run history (one session per flow)
//	schedule  an agent's scheduled-prompt deliveries (one per agent)
//	heartbeat an agent's autonomous wake turns
//
// SourceID links the session back to the entity that owns it (a task or flow id);
// it is empty for plain chat and for agent-keyed kinds (schedule/heartbeat). This
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

	// Conversation compaction state (see internal/conversation).
	Summary         string `json:"summary"`
	SummaryMsgCount int    `json:"summaryMsgCount"`

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
	// Attachments are user-supplied files (or pasted long text) sent with this
	// message. Stored on user turns; empty for assistant/system. The files live
	// under the workspace's uploads/ directory so agents can read them via their
	// sandboxed read_file tool (see Attachment.RelPath).
	Attachments []Attachment `json:"attachments,omitempty"`
	CreatedAt   int64        `json:"createdAt"`
}
