package db

import (
	"context"
	"testing"
)

func openReviewBounceDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// TestMoveTaskCountsReviewBounce is the move_task path: review → a working
// column is a FAILED verification round.
func TestMoveTaskCountsReviewBounce(t *testing.T) {
	d := openReviewBounceDB(t)
	ctx := context.Background()
	task, err := d.CreateTask(ctx, Task{Title: "tool path", BoardState: BoardInProgress})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for round := 1; round <= 2; round++ {
		if err := d.MoveTask(ctx, task.ID, BoardReview); err != nil {
			t.Fatalf("move to review: %v", err)
		}
		if err := d.MoveTask(ctx, task.ID, BoardInProgress); err != nil {
			t.Fatalf("move back: %v", err)
		}
		got, err := d.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.ReviewBounces != round {
			t.Fatalf("after round %d: reviewBounces = %d, want %d", round, got.ReviewBounces, round)
		}
	}
}

// TestUpdateTaskCountsReviewBounce covers the write path a CARD DRAG takes:
// PUT /api/tasks/{id} → UpdateTask, not MoveTask. Counting only in MoveTask left
// the badge invisible to every move a human makes.
func TestUpdateTaskCountsReviewBounce(t *testing.T) {
	d := openReviewBounceDB(t)
	ctx := context.Background()
	task, err := d.CreateTask(ctx, Task{Title: "drag path", BoardState: BoardInProgress})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for round := 1; round <= 2; round++ {
		task.BoardState = BoardReview
		if err := d.UpdateTask(ctx, task); err != nil {
			t.Fatalf("update to review: %v", err)
		}
		task.BoardState = BoardInProgress
		if err := d.UpdateTask(ctx, task); err != nil {
			t.Fatalf("update back: %v", err)
		}
		got, err := d.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.ReviewBounces != round {
			t.Fatalf("after round %d: reviewBounces = %d, want %d", round, got.ReviewBounces, round)
		}
	}
}

// TestUpdateTaskIgnoresClientReviewBounces: the field is server-owned. A client
// that echoes a stale (or invented) count back must not be able to set it.
func TestUpdateTaskIgnoresClientReviewBounces(t *testing.T) {
	d := openReviewBounceDB(t)
	ctx := context.Background()
	task, err := d.CreateTask(ctx, Task{Title: "server owned", BoardState: BoardInProgress})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	task.ReviewBounces = 99
	task.Title = "renamed"
	if err := d.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ReviewBounces != 0 {
		t.Fatalf("client-supplied reviewBounces was accepted: %d", got.ReviewBounces)
	}
	if got.Title != "renamed" {
		t.Fatalf("the rest of the update must still apply, title = %q", got.Title)
	}
}

// TestReviewToDoneIsNotABounce: a PASS is not a failed round, on either path.
func TestReviewToDoneIsNotABounce(t *testing.T) {
	d := openReviewBounceDB(t)
	ctx := context.Background()
	moved, err := d.CreateTask(ctx, Task{Title: "passes via move", BoardState: BoardReview})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := d.MoveTask(ctx, moved.ID, BoardDone); err != nil {
		t.Fatalf("move to done: %v", err)
	}
	updated, err := d.CreateTask(ctx, Task{Title: "passes via update", BoardState: BoardReview})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated.BoardState = BoardDone
	if err := d.UpdateTask(ctx, updated); err != nil {
		t.Fatalf("update to done: %v", err)
	}
	for _, id := range []string{moved.ID, updated.ID} {
		got, err := d.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if got.ReviewBounces != 0 {
			t.Fatalf("%s: review→done counted as a failed round (%d)", id, got.ReviewBounces)
		}
	}
}
