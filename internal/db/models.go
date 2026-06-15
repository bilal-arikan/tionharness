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

	// Heartbeat (autonomous wake) configuration.
	HeartbeatEnabled     bool   `json:"heartbeatEnabled"`
	HeartbeatIntervalSec int    `json:"heartbeatIntervalSec"`
	HeartbeatPrompt      string `json:"heartbeatPrompt"`

	// Daily spend caps for autonomous calls (0 = unlimited).
	DailyCallLimit  int `json:"dailyCallLimit"`
	DailyTokenLimit int `json:"dailyTokenLimit"`

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

	// Conversation compaction state (see internal/conversation).
	Summary         string `json:"summary"`
	SummaryMsgCount int    `json:"summaryMsgCount"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// Message is a single turn within a session.
type Message struct {
	ID               string `json:"id"`
	SessionID        string `json:"sessionId"`
	Role             string `json:"role"` // system | user | assistant | tool
	Text             string `json:"text"`
	ToolCalls        string `json:"toolCalls"`
	ReasoningContent string `json:"reasoningContent"`
	CreatedAt        int64  `json:"createdAt"`
}
