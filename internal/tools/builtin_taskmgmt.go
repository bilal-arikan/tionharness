package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// Task (kanban board) self-management tools let an agent read the board and
// create, edit, move and delete tasks in its workspace — so an agent can track
// and update work as a passive status board (e.g. moving a task it finished to
// "done"). The board never executes tasks; flows, schedules and agent sessions
// do the work and reflect status here. Agents act on the user's board, so
// read/create/edit/move/delete are all allowed on ANY task (including
// user-created ones). Task.CreatedBy is still stamped for provenance/display,
// but no longer gates deletion.

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
		Description: "List the tasks on the kanban board in this workspace (id, title, boardState, ownerAgentId, flowId, priority, tags, artifactIds, last run status, and whether each was created by an agent). artifactIds are workspace artifacts attached to the card (e.g. a plan) — read one with read_artifact. You can edit, move and delete ANY task. Built-in board columns are: pbi, todo, in_progress, review, done, failed — this workspace may also define custom columns; check existing tasks' boardState values or the board UI to see them.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ListTasksTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	tasks, err := t.d.db.ListActiveTasks(ctx)
	if err != nil {
		return "", err
	}
	type row struct {
		ID             string   `json:"id"`
		Title          string   `json:"title"`
		BoardState     string   `json:"boardState"`
		OwnerAgentID   string   `json:"ownerAgentId,omitempty"`
		FlowID         string   `json:"flowId,omitempty"`
		Priority       string   `json:"priority,omitempty"`
		Tags           []string `json:"tags,omitempty"`
		ArtifactIDs    []string `json:"artifactIds,omitempty"`
		LastRunStatus  string   `json:"lastRunStatus,omitempty"`
		CreatedByAgent bool     `json:"createdByAgent"`
	}
	out := make([]row, 0, len(tasks))
	for _, tk := range tasks {
		out = append(out, row{
			ID:             tk.ID,
			Title:          tk.Title,
			BoardState:     tk.BoardState,
			OwnerAgentID:   tk.OwnerAgentID,
			FlowID:         tk.FlowID,
			Priority:       tk.Priority,
			Tags:           tk.Tags,
			ArtifactIDs:    tk.ArtifactIDs,
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
		Name:        "create_task",
		Description: "Create a task on the kanban board. Provide a prompt (the instruction run by the owner agent) and/or a flowId (the task runs that orchestration flow instead, with the prompt as its input). Optionally set title (auto-generated from prompt when omitted), description, ownerAgentId, boardState (default todo), dependencies (JSON array of task IDs that must complete before this one), priority (critical/high/medium/low) and tags (string array). The task is tagged as created by you. Returns the new task id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"title":{"type":"string","description":"Short title (auto-generated from prompt when omitted)"},
				"prompt":{"type":"string","description":"Instruction delivered to the owner agent (or used as flow input)"},
				"description":{"type":"string"},
				"ownerAgentId":{"type":"string","description":"Agent that runs the task (see list_agents); not required for flow-backed tasks"},
				"flowId":{"type":"string","description":"When set, running the task executes this flow (see list_flows)"},
				"boardState":{"type":"string","description":"Initial column key (default todo). Built-in: pbi, todo, in_progress, review, done, failed — but a workspace may define custom columns (see list_tasks description or the board UI); any lowercase letters/digits/underscores key is accepted."},
				"dependencies":{"type":"string","description":"JSON array of task IDs that must complete before this task, e.g. [\"id1\",\"id2\"]"},
				"priority":{"type":"string","enum":["critical","high","medium","low"],"description":"Task priority (optional)"},
				"tags":{"type":"array","items":{"type":"string"},"description":"Free-form labels (optional)"},
				"artifactIds":{"type":"array","items":{"type":"string"},"description":"Workspace artifact ids to attach to this card (optional). Read one with read_artifact."}
			},
			"required":[],
			"additionalProperties":false
		}`),
	}
}

func (t CreateTaskTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Title        string   `json:"title"`
		Prompt       string   `json:"prompt"`
		Description  string   `json:"description"`
		OwnerAgentID string   `json:"ownerAgentId"`
		FlowID       string   `json:"flowId"`
		BoardState   string   `json:"boardState"`
		Dependencies string   `json:"dependencies"`
		Priority     string   `json:"priority"`
		Tags         []string `json:"tags"`
		ArtifactIDs  []string `json:"artifactIds"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.Title == "" && in.Prompt == "" && in.FlowID == "" {
		return "", fmt.Errorf("provide at least one of: prompt, title, flowId")
	}
	if in.BoardState != "" && !db.IsValidBoardKey(in.BoardState) {
		return "", enumErr("boardState", in.BoardState, "pbi", "todo", "in_progress", "review", "done", "failed", "or a workspace custom column key (lowercase letters/digits/underscores)")
	}
	if !db.ValidPriority(in.Priority) {
		return "", enumErr("priority", in.Priority, "critical", "high", "medium", "low")
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
		Dependencies: in.Dependencies,
		Priority:     in.Priority,
		Tags:         in.Tags,
		ArtifactIDs:  in.ArtifactIDs,
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
		Name:        "update_task",
		Description: "Edit a task on the board. Pass the task id and the fields to change (title, prompt, description, ownerAgentId, flowId, boardState, dependencies, priority, tags, artifactIds). Use artifactIds to attach workspace artifacts to the card (e.g. link a plan artifact so a later stage reads it by id instead of matching on title). To change only the column, prefer move_task. Allowed on any task.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The task id (see list_tasks)"},
				"title":{"type":"string"},
				"prompt":{"type":"string"},
				"description":{"type":"string"},
				"ownerAgentId":{"type":"string"},
				"flowId":{"type":"string","description":"Set to empty string to unlink the flow"},
				"boardState":{"type":"string","description":"Column key. Built-in: pbi, todo, in_progress, review, done, failed — plus any workspace custom column key."},
				"dependencies":{"type":"string","description":"JSON array of task IDs this task depends on, e.g. [\"id1\",\"id2\"]. Pass [] to clear."},
				"priority":{"type":"string","enum":["critical","high","medium","low",""],"description":"Priority ('' clears it)"},
				"tags":{"type":"array","items":{"type":"string"},"description":"Free-form labels (replaces the set; [] clears)"},
				"artifactIds":{"type":"array","items":{"type":"string"},"description":"Workspace artifact ids attached to this card (replaces the set; [] clears). Read one with read_artifact."}
			},
			"required":["id"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			// Move column (or prefer move_task for column-only changes).
			json.RawMessage(`{"id":"tsk_77","boardState":"in_progress"}`),
			// Set dependencies: a JSON ARRAY STRING of task ids ([] clears).
			json.RawMessage(`{"id":"tsk_77","dependencies":"[\"tsk_12\",\"tsk_34\"]"}`),
			// Unlink the flow by passing an empty string.
			json.RawMessage(`{"id":"tsk_77","flowId":""}`),
		},
	}
}

func (t UpdateTaskTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID           string    `json:"id"`
		Title        *string   `json:"title"`
		Prompt       *string   `json:"prompt"`
		Description  *string   `json:"description"`
		OwnerAgentID *string   `json:"ownerAgentId"`
		FlowID       *string   `json:"flowId"`
		BoardState   *string   `json:"boardState"`
		Dependencies *string   `json:"dependencies"`
		Priority     *string   `json:"priority"`
		Tags         *[]string `json:"tags"`
		ArtifactIDs  *[]string `json:"artifactIds"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
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
		if !db.IsValidBoardKey(*in.BoardState) {
			return "", enumErr("boardState", *in.BoardState, "pbi", "todo", "in_progress", "review", "done", "failed", "or a workspace custom column key (lowercase letters/digits/underscores)")
		}
		cur.BoardState = *in.BoardState
	}
	if in.Dependencies != nil {
		cur.Dependencies = *in.Dependencies
	}
	if in.Priority != nil {
		if !db.ValidPriority(*in.Priority) {
			return "", enumErr("priority", *in.Priority, "critical", "high", "medium", "low")
		}
		cur.Priority = *in.Priority
	}
	if in.Tags != nil {
		cur.Tags = *in.Tags
	}
	if in.ArtifactIDs != nil {
		cur.ArtifactIDs = *in.ArtifactIDs
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
		Name:        "move_task",
		Description: "Move a task to a different board column. Built-in columns: pbi, todo, in_progress, review, done, failed — a workspace may also define custom columns. Allowed on any task.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The task id (see list_tasks)"},
				"boardState":{"type":"string","description":"Column key. Built-in: pbi, todo, in_progress, review, done, failed — plus any workspace custom column key."}
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
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if !db.IsValidBoardKey(in.BoardState) {
		return "", enumErr("boardState", in.BoardState, "pbi", "todo", "in_progress", "review", "done", "failed", "or a workspace custom column key (lowercase letters/digits/underscores)")
	}
	if err := t.d.db.MoveTask(ctx, in.ID, in.BoardState); err != nil {
		return "", fmt.Errorf("move task: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "boardState": in.BoardState, "action": "moved"})
	return string(b), nil
}

// ---- delete_task ----

// DeleteTaskTool removes any task from the board (user- or agent-created).
type DeleteTaskTool struct{ d taskDeps }

// NewDeleteTaskTool constructs delete_task.
func NewDeleteTaskTool(database *db.DB, actorID string) DeleteTaskTool {
	return DeleteTaskTool{d: taskDeps{db: database, actorID: actorID}}
}

func (DeleteTaskTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_task",
		Description: "Delete a task from the kanban board (any task, including ones created by the user). Pass the task id. This is irreversible — the task is removed from the board.",
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
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if _, err := t.d.db.GetTask(ctx, in.ID); err != nil {
		return "", fmt.Errorf("no task with id %q (use list_tasks)", in.ID)
	}
	if err := t.d.db.DeleteTask(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete task: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}
