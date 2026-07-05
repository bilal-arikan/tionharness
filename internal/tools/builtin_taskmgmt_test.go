package tools

import (
	"context"
	"encoding/json"
	"strings"
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
	if _, err := move.Call(ctx, json.RawMessage(`{"id":"`+tk.ID+`","boardState":"nope"}`)); err == nil {
		t.Fatal("expected invalid boardState to be rejected")
	}
	if _, err := move.Call(ctx, json.RawMessage(`{"id":"`+tk.ID+`","boardState":"done"}`)); err != nil {
		t.Fatalf("move_task: %v", err)
	}
	got, _ := d.GetTask(ctx, tk.ID)
	if got.BoardState != db.BoardDone {
		t.Fatalf("BoardState = %q, want done", got.BoardState)
	}
}

// TestDeleteTaskGuard verifies only agent-created tasks can be deleted.
func TestDeleteTaskGuard(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"

	userTask, _ := d.CreateTask(ctx, db.Task{Title: "User", Prompt: "x"}) // CreatedBy == ""
	del := NewDeleteTaskTool(d, actor)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+userTask.ID+`"}`)); err == nil {
		t.Fatal("expected delete of user-created task to be rejected")
	} else if !strings.Contains(err.Error(), "created by the user") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := d.GetTask(ctx, userTask.ID); err != nil {
		t.Fatalf("user task should still exist: %v", err)
	}

	agentTask, _ := d.CreateTask(ctx, db.Task{Title: "Agent", Prompt: "y", CreatedBy: actor})
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+agentTask.ID+`"}`)); err != nil {
		t.Fatalf("delete of agent-created task should succeed: %v", err)
	}
	if _, err := d.GetTask(ctx, agentTask.ID); err == nil {
		t.Fatal("agent task should be gone")
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
