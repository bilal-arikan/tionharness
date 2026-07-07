package db

import (
	"context"
	"sync"
	"testing"
)

// TestBoardHookFires verifies the board hook is invoked for every kind of card
// change (create/move/update/delete) with the correct op and column context, and
// that a no-op move (same column) does not fire.
func TestBoardHookFires(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	var mu sync.Mutex
	var events []BoardChangeEvent
	d.SetBoardHook(func(ev BoardChangeEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})
	drain := func() []BoardChangeEvent {
		mu.Lock()
		defer mu.Unlock()
		out := events
		events = nil
		return out
	}

	// Create → BoardOpCreate with ToState = initial column.
	task, err := d.CreateTask(ctx, Task{Title: "Card", BoardState: BoardTodo})
	if err != nil {
		t.Fatal(err)
	}
	if ev := drain(); len(ev) != 1 || ev[0].Op != BoardOpCreate || ev[0].ToState != BoardTodo {
		t.Fatalf("create event = %+v", ev)
	}

	// Move → BoardOpMove with From/To set.
	if err := d.MoveTask(ctx, task.ID, BoardInProgress); err != nil {
		t.Fatal(err)
	}
	if ev := drain(); len(ev) != 1 || ev[0].Op != BoardOpMove ||
		ev[0].FromState != BoardTodo || ev[0].ToState != BoardInProgress {
		t.Fatalf("move event = %+v", ev)
	}

	// No-op move (same column) → no event.
	if err := d.MoveTask(ctx, task.ID, BoardInProgress); err != nil {
		t.Fatal(err)
	}
	if ev := drain(); len(ev) != 0 {
		t.Fatalf("no-op move should not fire, got %+v", ev)
	}

	// Update that changes the board → BoardOpMove; a non-board edit → BoardOpUpdate.
	task, _ = d.GetTask(ctx, task.ID)
	task.BoardState = BoardDone
	if err := d.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if ev := drain(); len(ev) != 1 || ev[0].Op != BoardOpMove || ev[0].ToState != BoardDone {
		t.Fatalf("update-move event = %+v", ev)
	}
	task, _ = d.GetTask(ctx, task.ID)
	task.Title = "Renamed"
	if err := d.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if ev := drain(); len(ev) != 1 || ev[0].Op != BoardOpUpdate {
		t.Fatalf("update event = %+v", ev)
	}

	// Delete → BoardOpDelete with FromState = last column.
	if err := d.DeleteTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if ev := drain(); len(ev) != 1 || ev[0].Op != BoardOpDelete || ev[0].FromState != BoardDone {
		t.Fatalf("delete event = %+v", ev)
	}
}
