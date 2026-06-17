package db

// Board states for a task. These drive the kanban columns in the UI.
const (
	BoardTodo       = "todo"
	BoardInProgress = "in_progress"
	BoardReview     = "review"
	BoardDone       = "done"
	BoardFailed     = "failed"
)

// ValidBoardState reports whether s is a recognised board column.
func ValidBoardState(s string) bool {
	switch s {
	case BoardTodo, BoardInProgress, BoardReview, BoardDone, BoardFailed:
		return true
	}
	return false
}

// Task is a unit of work on the board, optionally owned by an agent. When run,
// the owner agent is given Prompt and the textual result is stored as a Run.
type Task struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Prompt        string `json:"prompt"`
	OwnerAgentID  string `json:"ownerAgentId"`
	BoardState    string `json:"boardState"`
	Dependencies  string `json:"dependencies"` // JSON array of task ids
	LastRunID     string `json:"lastRunId"`
	LastRunStatus string `json:"lastRunStatus"`
	LastRunAt     int64  `json:"lastRunAt"`
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
}

// Schedule fires on a cron expression and either runs a linked task or delivers
// a standalone prompt to its agent.
type Schedule struct {
	ID                 string `json:"id"`
	AgentID            string `json:"agentId"`
	TaskID             string `json:"taskId"` // empty for standalone prompt schedules
	CronExpr           string `json:"cronExpr"`
	Prompt             string `json:"prompt"`
	NextRunAt          int64  `json:"nextRunAt"`
	LastRunAt          int64  `json:"lastRunAt"`
	LastDeliveryStatus string `json:"lastDeliveryStatus"`
	LastDeliveryError  string `json:"lastDeliveryError"`
	Enabled            bool   `json:"enabled"`
	// CreatedBy is the ID of the agent that created this schedule via a
	// self-management tool ("" = created by the user). Agents may only
	// edit/delete agent-created schedules.
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// Run statuses.
const (
	RunPending = "pending"
	RunRunning = "running"
	RunSuccess = "success"
	RunFailure = "failure"
)

// Run is a single execution attempt of a task (or a scheduled delivery).
type Run struct {
	ID        string `json:"id"`
	TaskID    string `json:"taskId"`
	AgentID   string `json:"agentId"`
	Status    string `json:"status"`
	Trigger   string `json:"trigger"` // manual | schedule | dependency
	Output    string `json:"output"`
	Error     string `json:"error"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}
