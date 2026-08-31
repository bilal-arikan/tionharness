package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
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

func TestCreateTaskDescriptionOnlyUsesRuntimeTitler(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	titled := make(chan struct{}, 1)
	create := NewCreateTaskTool(d, "actor-1", func(_ context.Context, owner, source string) (string, error) {
		if owner == "" || source != "Açıklamadan gelen kart" {
			t.Errorf("titler args = %q, %q", owner, source)
		}
		titled <- struct{}{}
		return "AI başlık", nil
	})
	out, err := create.Call(ctx, json.RawMessage(`{"description":"Açıklamadan gelen kart","ownerAgentId":"AG1"}`))
	if err == nil {
		t.Fatal("unknown owner must still be validated")
	}
	agent, err := d.CreateAgent(ctx, db.Agent{Name: "Owner"})
	if err != nil {
		t.Fatal(err)
	}
	out, err = create.Call(ctx, json.RawMessage(`{"description":"Açıklamadan gelen kart","ownerAgentId":"`+agent.ID+`"}`))
	if err != nil {
		t.Fatalf("description-only create: %v", err)
	}
	var res struct{ ID string }
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	select {
	case <-titled:
	case <-time.After(time.Second):
		t.Fatal("background titler was not called")
	}
	deadline := time.Now().Add(time.Second)
	for {
		got, err := d.GetTask(ctx, res.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Title == "AI başlık" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("title stayed %q", got.Title)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTaskToolsReadActiveAndArchivedSeparately(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	active, _ := d.CreateTask(ctx, db.Task{Title: "Active", Description: "full body", Dependencies: `["dep"]`})
	archived, _ := d.CreateTask(ctx, db.Task{Title: "Archived"})
	if err := d.SetTaskArchived(ctx, archived.ID, true); err != nil {
		t.Fatal(err)
	}

	list := NewListTasksTool(d, "actor-1")
	activeOut, err := list.Call(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(activeOut, active.ID) || strings.Contains(activeOut, archived.ID) || !strings.Contains(activeOut, `"dependencies":"[\"dep\"]"`) {
		t.Fatalf("active list mismatch: %s", activeOut)
	}
	archivedOut, err := list.Call(ctx, json.RawMessage(`{"archived":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(archivedOut, archived.ID) || strings.Contains(archivedOut, active.ID) {
		t.Fatalf("archived list mismatch: %s", archivedOut)
	}

	get := NewGetTaskTool(d, "actor-1")
	full, err := get.Call(ctx, json.RawMessage(`{"id":"`+active.ID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full, `"description":"full body"`) || !strings.Contains(full, `"createdAt"`) {
		t.Fatalf("get_task did not return full card: %s", full)
	}

	setArchived := NewSetArchivedTaskTool(d, "actor-1")
	if _, err := setArchived.Call(ctx, json.RawMessage(`{"id":"`+active.ID+`","archived":true}`)); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetTask(ctx, active.ID)
	if !got.Archived {
		t.Fatal("set_archived_task did not archive card")
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
