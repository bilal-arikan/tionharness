package db

import (
	"context"
	"testing"
)

// TestSetTaskArchivedHidesFromActiveBoard verifies archiving is a reversible
// soft-hide: an archived task drops out of ListActiveTasks but stays in ListTasks,
// unarchiving restores it, and a no-op archive neither errors nor duplicates.
func TestSetTaskArchivedHidesFromActiveBoard(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	a, err := d.CreateTask(ctx, Task{Title: "Keep", BoardState: BoardDone})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateTask(ctx, Task{Title: "Active", BoardState: BoardTodo}); err != nil {
		t.Fatal(err)
	}

	active, _ := d.ListActiveTasks(ctx)
	if len(active) != 2 {
		t.Fatalf("before archive: want 2 active, got %d", len(active))
	}

	if err := d.SetTaskArchived(ctx, a.ID, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	active, _ = d.ListActiveTasks(ctx)
	if len(active) != 1 || active[0].Title != "Active" {
		t.Fatalf("after archive: want only Active, got %+v", active)
	}
	all, _ := d.ListTasks(ctx)
	if len(all) != 2 {
		t.Fatalf("ListTasks must still return archived: got %d", len(all))
	}
	got, _ := d.GetTask(ctx, a.ID)
	if !got.Archived {
		t.Fatal("archived flag not persisted")
	}

	// No-op archive (already archived) is a clean no-op.
	if err := d.SetTaskArchived(ctx, a.ID, true); err != nil {
		t.Fatalf("no-op archive: %v", err)
	}

	// Unarchive restores it to the active board.
	if err := d.SetTaskArchived(ctx, a.ID, false); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	active, _ = d.ListActiveTasks(ctx)
	if len(active) != 2 {
		t.Fatalf("after unarchive: want 2 active, got %d", len(active))
	}
}

// TestSetTaskArchivedPreservedByUpdate proves a normal UpdateTask (an edit that
// doesn't touch the flag) does not clobber Archived — archiving lives solely in
// SetTaskArchived.
func TestSetTaskArchivedPreservedByUpdate(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a, err := d.CreateTask(ctx, Task{Title: "Card", BoardState: BoardDone})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetTaskArchived(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	cur, _ := d.GetTask(ctx, a.ID)
	cur.Title = "Renamed"
	if err := d.UpdateTask(ctx, cur); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetTask(ctx, a.ID)
	if !got.Archived {
		t.Fatal("UpdateTask clobbered Archived")
	}
	if got.Title != "Renamed" {
		t.Fatalf("update did not apply: %q", got.Title)
	}
}
