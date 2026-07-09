package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestCreateTaskStampsCreatedBy verifies create_task tags the new task with the
// acting agent's id and lands it in the todo column by default.
func TestCreateTaskStampsCreatedBy(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"

	create := NewCreateTaskTool(d, actor)
	out, err := create.Call(ctx, json.RawMessage(`{"prompt":"do the thing"}`))
	if err != nil {
		t.Fatalf("create_task: %v", err)
	}
	var res struct{ ID string }
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := d.GetTask(ctx, res.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.CreatedBy != actor {
		t.Fatalf("CreatedBy = %q, want %q", got.CreatedBy, actor)
	}
	if got.BoardState != db.BoardTodo {
		t.Fatalf("BoardState = %q, want %q", got.BoardState, db.BoardTodo)
	}
}

// TestMoveTaskChangesColumn verifies move_task validates the column and persists.
func TestMoveTaskChangesColumn(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	tk, _ := d.CreateTask(ctx, db.Task{Title: "T", Prompt: "p"})

	move := NewMoveTaskTool(d, "actor-1")
	if _, err := move.Call(ctx, json.RawMessage(`{"id":"`+tk.ID+`","boardState":"Not Valid!"}`)); err == nil {
		t.Fatal("expected invalid boardState to be rejected")
	}
	// A custom workspace column key (lowercase/digits/underscores) is accepted —
	// move_task no longer restricts boardState to the five built-in columns.
	if _, err := move.Call(ctx, json.RawMessage(`{"id":"`+tk.ID+`","boardState":"pbi"}`)); err != nil {
		t.Fatalf("move_task to custom column key should succeed: %v", err)
	}
	if _, err := move.Call(ctx, json.RawMessage(`{"id":"`+tk.ID+`","boardState":"done"}`)); err != nil {
		t.Fatalf("move_task: %v", err)
	}
	got, _ := d.GetTask(ctx, tk.ID)
	if got.BoardState != db.BoardDone {
		t.Fatalf("BoardState = %q, want done", got.BoardState)
	}
}

// TestDeleteTaskAny verifies an agent can delete ANY task — both user-created
// and agent-created — and that a missing id errors.
func TestDeleteTaskAny(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"
	del := NewDeleteTaskTool(d, actor)

	// User-created task (CreatedBy == "") is now deletable by an agent.
	userTask, _ := d.CreateTask(ctx, db.Task{Title: "User", Prompt: "x"})
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+userTask.ID+`"}`)); err != nil {
		t.Fatalf("delete of user-created task should succeed: %v", err)
	}
	if _, err := d.GetTask(ctx, userTask.ID); err == nil {
		t.Fatal("user task should be gone")
	}

	// Agent-created task is deletable too.
	agentTask, _ := d.CreateTask(ctx, db.Task{Title: "Agent", Prompt: "y", CreatedBy: actor})
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+agentTask.ID+`"}`)); err != nil {
		t.Fatalf("delete of agent-created task should succeed: %v", err)
	}
	if _, err := d.GetTask(ctx, agentTask.ID); err == nil {
		t.Fatal("agent task should be gone")
	}

	// Unknown id is a clear error, not a silent no-op.
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"tsk_missing"}`)); err == nil {
		t.Fatal("expected error deleting a non-existent task")
	}
}

// TestUpdateTaskFlowValidation verifies update_task rejects an unknown flow id.
func TestUpdateTaskFlowValidation(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	tk, _ := d.CreateTask(ctx, db.Task{Title: "T", Prompt: "p"})

	upd := NewUpdateTaskTool(d, "actor-1")
	if _, err := upd.Call(ctx, json.RawMessage(`{"id":"`+tk.ID+`","flowId":"nonexistent"}`)); err == nil {
		t.Fatal("expected unknown flowId to be rejected")
	}
}
