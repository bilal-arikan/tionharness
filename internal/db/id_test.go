package db

import (
	"context"
	"testing"
)

// TestNextIDMonotonicAndNoReuse verifies the human-readable id scheme:
// prefixed, monotonic, and never reused across deletions or a reopen.
func TestNextIDMonotonicAndNoReuse(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	t1, err := d.CreateTask(ctx, Task{Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	t2, err := d.CreateTask(ctx, Task{Title: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if t1.ID != "TSK1" || t2.ID != "TSK2" {
		t.Fatalf("expected TSK1/TSK2, got %s/%s", t1.ID, t2.ID)
	}

	// Distinct prefixes keep independent counters.
	a1, err := d.CreateAgent(ctx, Agent{Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if a1.ID != "AGT1" {
		t.Fatalf("expected AGT1, got %s", a1.ID)
	}

	// Delete the highest task, then reopen: the counter must NOT rewind.
	if err := d.DeleteTask(ctx, t2.ID); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()

	d2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t3, err := d2.CreateTask(ctx, Task{Title: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if t3.ID != "TSK3" {
		t.Fatalf("reuse/rewind detected: expected TSK3 after reopen, got %s", t3.ID)
	}
}
