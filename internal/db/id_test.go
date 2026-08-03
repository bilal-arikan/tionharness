package db

import (
	"context"
	"os"
	"strconv"
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

// TestNextIDReservesBlocksButStaysDense pins the id-block allocator: ids are
// handed out from a block claimed on disk up front (so counters.json is written
// once per idBlock creations, not once per id), yet a clean shutdown gives the
// unspent tail back so the numbering shows no gap.
func TestNextIDReservesBlocksButStaysDense(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// One creation must reserve a whole block on disk...
	if _, err := d.CreateTask(ctx, Task{Title: "a"}); err != nil {
		t.Fatal(err)
	}
	var onDisk map[string]int64
	if err := readJSONFile(d.dir(countersFile), &onDisk); err != nil {
		t.Fatalf("read counters: %v", err)
	}
	if onDisk["TSK"] != idBlock {
		t.Fatalf("reserved mark = %d, want %d (a full block)", onDisk["TSK"], idBlock)
	}
	// ...and the rest of the block must not touch the file again.
	stat, _ := os.Stat(d.dir(countersFile))
	for i := 0; i < idBlock-2; i++ {
		if _, err := d.CreateTask(ctx, Task{Title: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	stat2, _ := os.Stat(d.dir(countersFile))
	if !stat.ModTime().Equal(stat2.ModTime()) {
		t.Fatal("counters.json rewritten while the reserved block still had room")
	}

	// Clean shutdown hands the unspent tail back → next boot is dense.
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	d2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	next, err := d2.CreateTask(ctx, Task{Title: "next"})
	if err != nil {
		t.Fatal(err)
	}
	if next.ID != "TSK"+strconv.Itoa(idBlock) {
		t.Fatalf("after clean close got %s, want TSK%d (no gap)", next.ID, idBlock)
	}
}

// TestNextIDNeverReissuesAfterCrash is the invariant that matters more than
// density: a process that dies without Close forfeits its block's tail, but must
// never hand out a number a previous process already used.
func TestNextIDNeverReissuesAfterCrash(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := d.CreateTask(ctx, Task{Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash: drop the handle WITHOUT Close, so the reservation stands.
	d = nil
	_ = d

	d2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	next, err := d2.CreateTask(ctx, Task{Title: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == first.ID {
		t.Fatalf("reissued %s after a crash", next.ID)
	}
	if next.ID != "TSK"+strconv.Itoa(idBlock+1) {
		t.Fatalf("after crash got %s, want TSK%d (block tail forfeited)", next.ID, idBlock+1)
	}
}
