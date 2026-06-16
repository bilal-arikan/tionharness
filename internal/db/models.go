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

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// Session is a conversation thread belonging to an agent.
type Session struct {
	ID           string `json:"id"`
	AgentID      string `json:"agentId"`
	Kind         string `json:"kind"`
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
	Steps     string `json:"steps"`
	CreatedAt int64  `json:"createdAt"`
}
