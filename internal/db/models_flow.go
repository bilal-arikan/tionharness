package db

// Flow run statuses.
const (
	FlowRunning = "running"
	FlowSuccess = "success"
	FlowFailure = "failure"
)

// Flow is a reusable multi-agent orchestration protocol. Graph holds the JSON
// node graph (see internal/orchestration.Graph).
type Flow struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Graph string `json:"graph"` // JSON
	// Emoji is an optional cosmetic glyph shown wherever the flow is presented or
	// picked (flow list, title bar, schedule/automation flow badges, run views).
	// Persisted independently via SetFlowEmoji so it survives graph/name saves.
	Emoji string `json:"emoji,omitempty"`
	// Tags are free-form labels on the flow, editable by both the user (UI) and
	// agents (set_flow_tags). Organizational only (they do not drive automations —
	// only session tags do).
	Tags []string `json:"tags,omitempty"`
	// Seed, when non-empty, marks this flow as a shipped built-in default
	// provisioned by EnsureDefaultFlows. The value is the stable seed key; it
	// lets seeding skip an already-present default and lets the UI recognize a
	// default. User- and agent-created flows leave it "".
	Seed string `json:"seed,omitempty"`
	// CreatedBy is the ID of the agent that created this flow via a
	// self-management tool ("" = created by the user). Agents may only
	// edit/delete agent-created flows.
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// FlowRun is one execution instance of a flow. State is the restart-safe JSON
// snapshot the engine persists after each node.
type FlowRun struct {
	ID        string `json:"id"`
	FlowID    string `json:"flowId"`
	Status    string `json:"status"`
	Input     string `json:"input"`
	State     string `json:"state"` // JSON
	Output    string `json:"output"`
	Error     string `json:"error"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}
