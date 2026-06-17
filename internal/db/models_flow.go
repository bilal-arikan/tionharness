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
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Graph       string `json:"graph"` // JSON
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
