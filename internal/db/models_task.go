package db

// Board states for a task. These drive the default kanban columns in the UI.
const (
	BoardPBI        = "pbi"
	BoardTodo       = "todo"
	BoardInProgress = "in_progress"
	BoardReview     = "review"
	BoardDone       = "done"
	BoardFailed     = "failed"
)

// BoardColumnDef defines a kanban column with a display label and optional
// accent color. Key is the persistent string stored on tasks (boardState).
type BoardColumnDef struct {
	Key   string `json:"key"`   // unique slug: lowercase letters, digits, underscores
	Label string `json:"label"` // human-readable column name
	Color string `json:"color"` // hex accent (e.g. "#4f8cff") or "" for default
}

// DefaultBoardColumns returns the built-in column set used when a workspace
// has no custom column configuration.
func DefaultBoardColumns() []BoardColumnDef {
	return []BoardColumnDef{
		{Key: BoardPBI, Label: "PBI", Color: ""},
		{Key: BoardTodo, Label: "Yapılacak", Color: ""},
		{Key: BoardInProgress, Label: "Devam Eden", Color: ""},
		{Key: BoardReview, Label: "İnceleme", Color: ""},
		{Key: BoardDone, Label: "Bitti", Color: ""},
		{Key: BoardFailed, Label: "Başarısız", Color: ""},
	}
}

// ValidBoardState reports whether s is one of the six built-in board column keys.
func ValidBoardState(s string) bool {
	switch s {
	case BoardPBI, BoardTodo, BoardInProgress, BoardReview, BoardDone, BoardFailed:
		return true
	default:
		return false
	}
}

// Task priority levels (obsidian-pm compatible). Empty string = unset.
const (
	PriorityCritical = "critical"
	PriorityHigh     = "high"
	PriorityMedium   = "medium"
	PriorityLow      = "low"
)

// ValidPriority reports whether p is empty (unset) or a known priority level.
func ValidPriority(p string) bool {
	switch p {
	case "", PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow:
		return true
	default:
		return false
	}
}

// Task is a unit of work on the board, optionally owned by an agent. When run,
// the owner agent is given Prompt and the textual result is stored as a Run.
// Alternatively, when FlowID is set the task is "flow-backed": running it executes
// that orchestration flow (with Prompt as the flow input) instead of delivering a
// prompt to a single agent. Either path funnels through RunTask, so manual runs,
// cron schedules and any future dispatcher support flows uniformly.
type Task struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	OwnerAgentID string `json:"ownerAgentId"`
	FlowID       string `json:"flowId"` // when set, running the task executes this flow
	BoardState   string `json:"boardState"`
	Dependencies string `json:"dependencies"` // JSON array of task ids
	// Rich card attributes (obsidian-pm compatible). All optional; older task
	// files without them decode to zero values.
	Priority string   `json:"priority,omitempty"` // critical|high|medium|low ("" = unset)
	Tags     []string `json:"tags,omitempty"`     // free-form labels
	// ArtifactIDs references workspace artifacts attached to this card (files
	// dropped onto the card become artifacts, or existing artifacts linked from
	// the editor). Order is user-meaningful; ids that no longer resolve are
	// skipped by the UI. Independent of artifact lifecycle — deleting the task
	// drops the refs, it does not delete the artifacts.
	ArtifactIDs []string `json:"artifactIds,omitempty"`
	Progress    int      `json:"progress,omitempty"`  // 0..100
	StartDate   string   `json:"startDate,omitempty"` // YYYY-MM-DD
	DueDate     string   `json:"dueDate,omitempty"`   // YYYY-MM-DD
	// Archived hides a finished card from the active board without deleting it
	// (reversible, unlike DeleteTask). Set by SetTaskArchived — typically by the
	// "done → archive" board automation — and excluded by default from the board
	// list, the get_view board projection, and the list_tasks tool. The task file
	// and its runs are kept, so an archived card can be restored.
	Archived      bool   `json:"archived,omitempty"`
	LastRunID     string `json:"lastRunId"`
	LastRunStatus string `json:"lastRunStatus"`
	LastRunAt     int64  `json:"lastRunAt"`
	// CreatedBy is the ID of the agent that created this task via a
	// self-management tool ("" = created by the user). Agents may only
	// delete agent-created tasks (read/edit/move/run are allowed on any task).
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Schedule fires on a cron expression and delivers a standalone prompt to its
// agent. (Schedules are decoupled from the board: they do not run tasks.)
//
// Alternatively, when FlowID is set the schedule is "flow-backed": each fire runs
// that orchestration flow (with Prompt as the flow input) instead of delivering
// the prompt to a single agent. AgentID is then optional — the flow owns its own
// agents. Either path funnels through Scheduler.run, so cron ticks and manual
// "run now" support both uniformly.
type Schedule struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	AgentID  string `json:"agentId"`
	CronExpr string `json:"cronExpr"`
	Prompt   string `json:"prompt"`
	// FlowID, when set, makes this a flow-backed schedule: firing runs that flow
	// with Prompt as its input instead of delivering the prompt to AgentID.
	FlowID             string `json:"flowId,omitempty"`
	NextRunAt          int64  `json:"nextRunAt"`
	LastRunAt          int64  `json:"lastRunAt"`
	LastDeliveryStatus string `json:"lastDeliveryStatus"`
	LastDeliveryError  string `json:"lastDeliveryError"`
	Enabled            bool   `json:"enabled"`
	// Tags are free-form labels on the schedule, editable by both the user (UI) and
	// agents (set_schedule_tags). Organizational only (they do not drive
	// automations — only session tags do).
	Tags []string `json:"tags,omitempty"`
	// ExpiresAt is an optional end date (unix seconds). When > 0 the schedule
	// stops firing once the time passes — the next due cron tick is skipped and
	// the schedule is auto-disabled. 0 means "no end date" (runs indefinitely).
	ExpiresAt int64 `json:"expiresAt,omitempty"`
	// CreatedBy is the ID of the agent that created this schedule via a
	// self-management tool ("" = created by the user). Agents may only
	// edit/delete agent-created schedules.
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`

	// One-shot wake fields. When OneShot is true the schedule is NOT driven by a
	// cron expression; it fires exactly once at FireAt (unix seconds) and is then
	// removed. A wake re-delivers Prompt into SessionID (the originating chat
	// session) so the conversation visibly continues on its own — this backs the
	// schedule_wake tool (see _Docs/18-SCHEDULE-WAKE.md). Reason is the agent's
	// stated purpose, shown for context.
	OneShot   bool   `json:"oneShot,omitempty"`
	FireAt    int64  `json:"fireAt,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Run statuses.
const (
	RunPending = "pending"
	RunRunning = "running"
	RunSuccess = "success"
	RunFailure = "failure"
)

// Run is a single execution attempt of a task (or a scheduled delivery).
//
// SessionID links the run to the task's transcript session (see Session.Kind
// "task"): each run appends a user turn (the prompt/input) plus an assistant turn
// (the rich activity trace) to that session, so a board run is viewable in the
// same streamable transcript as a chat. MessageID is the assistant turn this run
// produced, for deep-linking straight to it.
type Run struct {
	ID        string `json:"id"`
	TaskID    string `json:"taskId"`
	AgentID   string `json:"agentId"`
	SessionID string `json:"sessionId,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	Status    string `json:"status"`
	Trigger   string `json:"trigger"` // manual | schedule | dependency
	Output    string `json:"output"`
	Error     string `json:"error"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}
