package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// Task (kanban board) self-management tools let an agent read the board and
// create, edit, move and delete tasks in its workspace — so an agent can track
// and update work as a passive status board (e.g. moving a task it finished to
// "done"). The board never executes tasks; flows, schedules and agent sessions
// do the work and reflect status here. Safety boundary: read/create/edit/move
// are allowed on ANY task (agents act on the user's board), but delete is
// restricted to tasks the agent itself created (provenance via Task.CreatedBy),
// so an agent can never throw away the user's work.

type taskDeps struct {
	db      *db.DB
	actorID string
}

// truncateForTool caps long text so a tool result stays compact.
func truncateForTool(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(truncated)"
}

// ---- list_tasks ----

// ListTasksTool returns the board as a compact list.
type ListTasksTool struct{ d taskDeps }

// NewListTasksTool constructs list_tasks.
func NewListTasksTool(database *db.DB, actorID string) ListTasksTool {
	return ListTasksTool{d: taskDeps{db: database, actorID: actorID}}
}

func (ListTasksTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "list_tasks",
		Description: "List the tasks on the kanban board in this workspace (id, title, boardState, ownerAgentId, flowId, last run status, and whether each was created by an agent and is therefore deletable by you). Board columns are: todo, in_progress, review, done, failed.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ListTasksTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	tasks, err := t.d.db.ListTasks(ctx)
	if err != nil {
		return "", err
	}
	type row struct {
		ID             string `json:"id"`
		Title          string `json:"title"`
		BoardState     string `json:"boardState"`
		OwnerAgentID   string `json:"ownerAgentId,omitempty"`
		FlowID         string `json:"flowId,omitempty"`
		LastRunStatus  string `json:"lastRunStatus,omitempty"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(tasks))
	for _, tk := range tasks {
		out = append(out, row{
			ID:             tk.ID,
			Title:          tk.Title,
			BoardState:     tk.BoardState,
			OwnerAgentID:   tk.OwnerAgentID,
			FlowID:         tk.FlowID,
			LastRunStatus:  tk.LastRunStatus,
			CreatedByAgent: tk.CreatedBy != "",
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ---- create_task ----

// CreateTaskTool adds a task to the board.
type CreateTaskTool struct{ d taskDeps }

// NewCreateTaskTool constructs create_task.
func NewCreateTaskTool(database *db.DB, actorID string) CreateTaskTool {
	return CreateTaskTool{d: taskDeps{db: database, actorID: actorID}}
}

func (CreateTaskTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "create_task",
		Description: "Create a task on the kanban board. Provide a prompt (the instruction run by the owner agent) and/or a flowId (the task runs that orchestration flow instead, with the prompt as its input). Optionally set title (auto-generated from prompt when omitted), description, ownerAgentId and boardState (default todo). The task is tagged as created by you. Returns the new task id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"title":{"type":"string","description":"Short title (auto-generated from prompt when omitted)"},
				"prompt":{"type":"string","description":"Instruction delivered to the owner agent (or used as flow input)"},
				"description":{"type":"string"},
				"ownerAgentId":{"type":"string","description":"Agent that runs the task (see list_agents); not required for flow-backed tasks"},
				"flowId":{"type":"string","description":"When set, running the task executes this flow (see list_flows)"},
				"boardState":{"type":"string","enum":["todo","in_progress","review","done","failed"],"description":"Initial column (default todo)"}
			},
			"required":[],
			"additionalProperties":false
		}`),
	}
}

func (t CreateTaskTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Title        string `json:"title"`
		Prompt       string `json:"prompt"`
		Description  string `json:"description"`
		OwnerAgentID string `json:"ownerAgentId"`
		FlowID       string `json:"flowId"`
		BoardState   string `json:"boardState"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.Title == "" && in.Prompt == "" && in.FlowID == "" {
		return "", fmt.Errorf("provide at least one of: prompt, title, flowId")
	}
	if in.BoardState != "" && !db.ValidBoardState(in.BoardState) {
		return "", fmt.Errorf("invalid boardState %q (todo|in_progress|review|done|failed)", in.BoardState)
	}
	if in.OwnerAgentID != "" {
		if _, err := t.d.db.GetAgent(ctx, in.OwnerAgentID); err != nil {
			return "", fmt.Errorf("no agent with id %q (use list_agents)", in.OwnerAgentID)
		}
	}
	if in.FlowID != "" {
		if _, err := t.d.db.GetFlow(ctx, in.FlowID); err != nil {
			return "", fmt.Errorf("no flow with id %q (use list_flows)", in.FlowID)
		}
	}
	title := in.Title
	if title == "" {
		title = truncateForTool(in.Prompt, 60)
	}
	created, err := t.d.db.CreateTask(ctx, db.Task{
		Title:        title,
		Prompt:       in.Prompt,
		Description:  in.Description,
		OwnerAgentID: in.OwnerAgentID,
		FlowID:       in.FlowID,
		BoardState:   in.BoardState,
		CreatedBy:    t.d.actorID,
	})
	if err != nil {
		return "", fmt.Errorf("create task: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": created.ID, "boardState": created.BoardState, "action": "created"})
	return string(b), nil
}

// ---- update_task ----

// UpdateTaskTool edits a task's fields (any task on the board).
type UpdateTaskTool struct{ d taskDeps }

// NewUpdateTaskTool constructs update_task.
func NewUpdateTaskTool(database *db.DB, actorID string) UpdateTaskTool {
	return UpdateTaskTool{d: taskDeps{db: database, actorID: actorID}}
}

func (UpdateTaskTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "update_task",
		Description: "Edit a task on the board. Pass the task id and the fields to change (title, prompt, description, ownerAgentId, flowId, boardState). To change only the column, prefer move_task. Allowed on any task.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The task id (see list_tasks)"},
				"title":{"type":"string"},
				"prompt":{"type":"string"},
				"description":{"type":"string"},
				"ownerAgentId":{"type":"string"},
				"flowId":{"type":"string","description":"Set to empty string to unlink the flow"},
				"boardState":{"type":"string","enum":["todo","in_progress","review","done","failed"]}
			},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t UpdateTaskTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID           string  `json:"id"`
		Title        *string `json:"title"`
		Prompt       *string `json:"prompt"`
		Description  *string `json:"description"`
		OwnerAgentID *string `json:"ownerAgentId"`
		FlowID       *string `json:"flowId"`
		BoardState   *string `json:"boardState"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	cur, err := t.d.db.GetTask(ctx, in.ID)
	if err != nil {
		return "", fmt.Errorf("no task with id %q (use list_tasks)", in.ID)
	}
	if in.Title != nil {
		cur.Title = *in.Title
	}
	if in.Prompt != nil {
		cur.Prompt = *in.Prompt
	}
	if in.Description != nil {
		cur.Description = *in.Description
	}
	if in.OwnerAgentID != nil {
		if *in.OwnerAgentID != "" {
			if _, err := t.d.db.GetAgent(ctx, *in.OwnerAgentID); err != nil {
				return "", fmt.Errorf("no agent with id %q", *in.OwnerAgentID)
			}
		}
		cur.OwnerAgentID = *in.OwnerAgentID
	}
	if in.FlowID != nil {
		if *in.FlowID != "" {
			if _, err := t.d.db.GetFlow(ctx, *in.FlowID); err != nil {
				return "", fmt.Errorf("no flow with id %q", *in.FlowID)
			}
		}
		cur.FlowID = *in.FlowID
	}
	if in.BoardState != nil {
		if !db.ValidBoardState(*in.BoardState) {
			return "", fmt.Errorf("invalid boardState %q", *in.BoardState)
		}
		cur.BoardState = *in.BoardState
	}
	if err := t.d.db.UpdateTask(ctx, cur); err != nil {
		return "", fmt.Errorf("update task: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "updated"})
	return string(b), nil
}

// ---- move_task ----

// MoveTaskTool changes only a task's board column.
type MoveTaskTool struct{ d taskDeps }

// NewMoveTaskTool constructs move_task.
func NewMoveTaskTool(database *db.DB, actorID string) MoveTaskTool {
	return MoveTaskTool{d: taskDeps{db: database, actorID: actorID}}
}

func (MoveTaskTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "move_task",
		Description: "Move a task to a different board column (todo, in_progress, review, done, failed). Allowed on any task.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The task id (see list_tasks)"},
				"boardState":{"type":"string","enum":["todo","in_progress","review","done","failed"]}
			},
			"required":["id","boardState"],
			"additionalProperties":false
		}`),
	}
}

func (t MoveTaskTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID         string `json:"id"`
		BoardState string `json:"boardState"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if !db.ValidBoardState(in.BoardState) {
		return "", fmt.Errorf("invalid boardState %q (todo|in_progress|review|done|failed)", in.BoardState)
	}
	if err := t.d.db.MoveTask(ctx, in.ID, in.BoardState); err != nil {
		return "", fmt.Errorf("move task: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "boardState": in.BoardState, "action": "moved"})
	return string(b), nil
}

// ---- delete_task ----

// DeleteTaskTool removes an agent-created task (provenance-enforced).
type DeleteTaskTool struct{ d taskDeps }

// NewDeleteTaskTool constructs delete_task.
func NewDeleteTaskTool(database *db.DB, actorID string) DeleteTaskTool {
	return DeleteTaskTool{d: taskDeps{db: database, actorID: actorID}}
}

func (DeleteTaskTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "delete_task",
		Description: "Delete an agent-created task (not one made by the user). Pass the task id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The task id (see list_tasks)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteTaskTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	cur, err := t.d.db.GetTask(ctx, in.ID)
	if err != nil {
		return "", fmt.Errorf("no task with id %q (use list_tasks)", in.ID)
	}
	if cur.CreatedBy == "" {
		return "", fmt.Errorf("task %q was created by the user and cannot be deleted by an agent", in.ID)
	}
	if err := t.d.db.DeleteTask(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete task: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}
