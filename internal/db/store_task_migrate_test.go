package db

import (
	"context"
	"testing"
)

// TestMigrateBoardColumns_Rename verifies that renaming a column (same index,
// different key) moves every task whose BoardState pointed at the old key onto
// the new key, instead of leaving them stranded off the board.
func TestMigrateBoardColumns_Rename(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	task, err := d.CreateTask(ctx, Task{Title: "Card", BoardState: BoardTodo})
	if err != nil {
		t.Fatal(err)
	}

	oldCols := DefaultBoardColumns()
	newCols := DefaultBoardColumns()
	for i := range newCols {
		if newCols[i].Key == BoardTodo {
			newCols[i].Key = "backlog"
		}
	}

	moved, err := d.MigrateBoardColumns(ctx, oldCols, newCols)
	if err != nil {
		t.Fatal(err)
	}
	if moved != 1 {
		t.Fatalf("moved = %d, want 1", moved)
	}
	got, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.BoardState != "backlog" {
		t.Fatalf("BoardState = %q, want %q", got.BoardState, "backlog")
	}
}

// TestMigrateBoardColumns_Delete verifies that deleting a column (no rename
// target) moves its tasks to the first remaining column rather than losing them.
func TestMigrateBoardColumns_Delete(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	task, err := d.CreateTask(ctx, Task{Title: "Card", BoardState: BoardFailed})
	if err != nil {
		t.Fatal(err)
	}

	oldCols := DefaultBoardColumns()
	var newCols []BoardColumnDef
	for _, c := range oldCols {
		if c.Key != BoardFailed {
			newCols = append(newCols, c)
		}
	}

	moved, err := d.MigrateBoardColumns(ctx, oldCols, newCols)
	if err != nil {
		t.Fatal(err)
	}
	if moved != 1 {
		t.Fatalf("moved = %d, want 1", moved)
	}
	got, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.BoardState != newCols[0].Key {
		t.Fatalf("BoardState = %q, want %q (first remaining column)", got.BoardState, newCols[0].Key)
	}

	all, err := d.ListActiveTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected the task to still be on the board, got %d tasks", len(all))
	}
}

// TestMigrateBoardColumns_NoChange verifies that a patch which does not
// rename or delete any column leaves every task's BoardState untouched.
func TestMigrateBoardColumns_NoChange(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	task, err := d.CreateTask(ctx, Task{Title: "Card", BoardState: BoardInProgress})
	if err != nil {
		t.Fatal(err)
	}

	oldCols := DefaultBoardColumns()
	newCols := DefaultBoardColumns()
	// Cosmetic-only change (label/color), keys identical — must not migrate anything.
	for i := range newCols {
		newCols[i].Color = "#123456"
	}

	moved, err := d.MigrateBoardColumns(ctx, oldCols, newCols)
	if err != nil {
		t.Fatal(err)
	}
	if moved != 0 {
		t.Fatalf("moved = %d, want 0", moved)
	}
	got, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.BoardState != BoardInProgress {
		t.Fatalf("BoardState = %q, want unchanged %q", got.BoardState, BoardInProgress)
	}
}
