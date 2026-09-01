package db

// Board states for a task. These drive the default kanban columns in the UI.
const (
	BoardPBI        = "pbi"
	BoardTodo       = "todo"
	BoardInProgress = "in_progress"
	BoardReview     = "review"
	BoardDone       = "done"
	BoardFailed     = "failed"
	BoardCancelled  = "iptal"
)

// ReviewRoundBudget is how many FAILED verification rounds a card may take
// before the coordinator must stop spawning reviewers and escalate to the user
// (see Task.ReviewBounces).
//
// Three is not arbitrary: rounds one and two are ordinary (a real defect found,
// then a real defect in the fix). By round four the pattern in SES2570 was that
// each new "fresh, independent" reviewer raised objections the previous rounds
// had already litigated — the loop was no longer converging on a defect, it was
// sampling opinions. That run took 7 hours and never produced a commit.
//
// It lives here, next to the field it bounds, because three layers read it and
// none may import another: the runtime (which injects the escalation block), the
// view projection (which flags the cards on the board), and — mirrored, not
// imported — the frontend badge (frontend/src/features/tasks/reviewGate.ts).
const ReviewRoundBudget = 3

const (
	WorktreeNone        = "none"
	WorktreeProvisioned = "provisioned"
	WorktreeMerged      = "merged"
	WorktreeDiscarded   = "discarded"
	WorktreeConflict    = "conflict"
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
		{Key: BoardCancelled, Label: "İptal", Color: ""},
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

// Task is a unit of work on the board, optionally owned by an agent.
//
// The board does NOT execute tasks: there is no dispatcher and, since the Run
// entity was removed, no execution record either. A card carries the intent —
// Prompt, or FlowID for a flow-backed one — and running it happens wherever the
// user or an agent takes it.
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
	// ReviewBounces counts how many times this card has returned from the review
	// column to a working column — i.e. how many verification rounds it has FAILED.
	// Maintained by MoveTask; it is the only durable record that a card is on a
	// verification treadmill, because each round is otherwise indistinguishable from
	// the first. A coordinator run (SES2570) spent seven hours cycling one card
	// through fresh reviewers, each finding new objections, with nothing counting
	// the rounds. Surfaced to coordinators as a hard escalation once it exceeds the
	// round budget (agent.ReviewRoundBudget).
	ReviewBounces int `json:"reviewBounces,omitempty"`
	// ArtifactIDs references workspace artifacts attached to this card (files
	// dropped onto the card become artifacts, or existing artifacts linked from
	// the editor). Order is user-meaningful; ids that no longer resolve are
	// skipped by the UI. Independent of artifact lifecycle — deleting the task
	// drops the refs, it does not delete the artifacts.
	ArtifactIDs []string `json:"artifactIds,omitempty"`
	// Worktree* fields are owned exclusively by the card lifecycle manager. The
	// removed session-level worktree feature must integrate with that manager if
	// it is ever restored; it must not create a second lifecycle owner.
	WorktreeBranch    string `json:"worktreeBranch,omitempty"`
	WorktreePath      string `json:"worktreePath,omitempty"`
	WorktreeBaseRef   string `json:"worktreeBaseRef,omitempty"`
	WorktreeState     string `json:"worktreeState,omitempty"` // none|provisioned|merged|discarded|conflict
	WorktreeLastError string `json:"worktreeLastError,omitempty"`
	// Archived hides a finished card from the active board without deleting it
	// (reversible, unlike DeleteTask). Set by SetTaskArchived — typically by the
	// "done → archive" board automation — and excluded by default from the board
	// list, the get_view board projection, and the list_tasks tool. The task file
	// is kept, so an archived card can be restored.
	Archived bool `json:"archived,omitempty"`
	// LastRun* are LEGACY and read-only: nothing sets them since the board stopped
	// executing tasks. They are kept because task files written by older builds
	// carry real values that the board view and list_tasks still surface — dropping
	// the fields would discard that history on the next write of each task.
	LastRunID     string `json:"lastRunId"`
	LastRunStatus string `json:"lastRunStatus"`
	LastRunAt     int64  `json:"lastRunAt"`
	// CreatedBy is the ID of the agent that created this task via a
	// self-management tool ("" = created by the user), for provenance/display.
	// It does not gate deletion: delete_task removes ANY task, including
	// user-created ones (read/edit/move/delete are all allowed on any task).
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Schedule session modes. An agent-backed schedule either REUSES the agent's one
// long-lived "schedule" thread so every fire is another turn in the same
// conversation (ScheduleSessionModeReuse — the original and default behavior), or
// opens a FRESH independent session per fire (ScheduleSessionModeSpawn), which
// keeps each run's context clean and stops the shared thread from growing without
// bound. SessionMode == "" is treated as reuse by EffectiveSessionMode, so rows
// written before the field existed keep their behavior.
const (
	ScheduleSessionModeReuse = "reuse"
	ScheduleSessionModeSpawn = "spawn"
)

// ValidScheduleSessionMode reports whether mode is empty (defaults to reuse) or a
// known schedule session mode.
func ValidScheduleSessionMode(mode string) bool {
	switch mode {
	case "", ScheduleSessionModeReuse, ScheduleSessionModeSpawn:
		return true
	default:
		return false
	}
}

// EffectiveSessionMode resolves the stored SessionMode to a concrete mode,
// mapping the empty default to reuse. Callers (the scheduler's fire path) should
// use this instead of repeating the `== "" → reuse` fallback. Value receiver (not
// pointer): callers pass Schedule around by value and the tests call it on a
// composite literal, which is not addressable.
func (sc Schedule) EffectiveSessionMode() string {
	if sc.SessionMode == "" {
		return ScheduleSessionModeReuse
	}
	return sc.SessionMode
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
	FlowID string `json:"flowId,omitempty"`
	// SessionMode selects, for an AGENT-backed schedule, whether each fire adds a
	// turn to the agent's shared "schedule" thread (ScheduleSessionModeReuse) or
	// opens a fresh session of its own (ScheduleSessionModeSpawn). Empty resolves to
	// reuse via EffectiveSessionMode. Ignored for flow-backed schedules (a flow
	// always records into its own per-run transcript).
	SessionMode        string `json:"sessionMode,omitempty"`
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
