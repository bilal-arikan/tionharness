package db

import (
	"context"
	"testing"
)

// TestFlowRun_RootOfEncodesEmptyAsSelf verifies the "empty RootRunID means self"
// encoding: a root run needs no second write to point at its own generated id,
// and runs created before lineage tracking read back as roots rather than as
// orphans of a missing tree.
func TestFlowRun_RootOfEncodesEmptyAsSelf(t *testing.T) {
	root := FlowRun{ID: "FRN1"}
	if got := root.RootOf(); got != "FRN1" {
		t.Errorf("root RootOf() = %q, want its own id", got)
	}
	if !root.IsRootRun() {
		t.Error("a run with no ParentRunID should be a root")
	}
	child := FlowRun{ID: "FRN2", ParentRunID: "FRN1", RootRunID: "FRN1"}
	if got := child.RootOf(); got != "FRN1" {
		t.Errorf("child RootOf() = %q, want %q", got, "FRN1")
	}
	if child.IsRootRun() {
		t.Error("a run with a ParentRunID should not be a root")
	}
}

// TestCreateFlowRun_PersistsLineage verifies the parent/node/root linkage
// survives a round-trip through the store.
func TestCreateFlowRun_PersistsLineage(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	parent, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW-parent"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if !parent.IsRootRun() || parent.RootOf() != parent.ID {
		t.Fatalf("a standalone run should be its own root, got parent=%q root=%q", parent.ParentRunID, parent.RootOf())
	}

	child, err := d.CreateFlowRun(ctx, FlowRun{
		FlowID: "FLW-child", ParentRunID: parent.ID, ParentNodeID: "sub-1", RootRunID: parent.ID,
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	got, err := d.GetFlowRun(ctx, child.ID)
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if got.ParentRunID != parent.ID || got.ParentNodeID != "sub-1" || got.RootOf() != parent.ID {
		t.Errorf("lineage not persisted: %+v", got)
	}
}

// TestListFlowRunTree_ReturnsWholeTreeOldestFirst verifies a grandchild several
// levels down still resolves via RootRunID (one scan, not a per-level walk), and
// that ordering puts a parent before its children so a caller can build the
// hierarchy in a single pass.
func TestListFlowRunTree_ReturnsWholeTreeOldestFirst(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	root, _ := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW-root"})
	child, _ := d.CreateFlowRun(ctx, FlowRun{
		FlowID: "FLW-child", ParentRunID: root.ID, ParentNodeID: "sub", RootRunID: root.ID,
	})
	// Two levels down: RootRunID still points at the TOP, not at the child.
	grand, _ := d.CreateFlowRun(ctx, FlowRun{
		FlowID: "FLW-grand", ParentRunID: child.ID, ParentNodeID: "sub", RootRunID: root.ID,
	})
	// An unrelated tree must not leak in.
	other, _ := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW-other"})

	tree, err := d.ListFlowRunTree(ctx, root.ID)
	if err != nil {
		t.Fatalf("list tree: %v", err)
	}
	if len(tree) != 3 {
		t.Fatalf("expected 3 runs in the tree, got %d", len(tree))
	}
	if tree[0].ID != root.ID || tree[1].ID != child.ID || tree[2].ID != grand.ID {
		t.Errorf("tree not oldest-first: %s, %s, %s", tree[0].ID, tree[1].ID, tree[2].ID)
	}
	for _, r := range tree {
		if r.ID == other.ID {
			t.Error("an unrelated run leaked into the tree")
		}
	}
}

// TestListRootFlowRuns_HidesChildren verifies the run list can exclude children
// so a composed flow does not flood it, while children stay first-class and
// remain fetchable on their own.
func TestListRootFlowRuns_HidesChildren(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	root, _ := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW-root"})
	child, _ := d.CreateFlowRun(ctx, FlowRun{
		FlowID: "FLW-child", ParentRunID: root.ID, RootRunID: root.ID,
	})

	all, _ := d.ListFlowRuns(ctx, "")
	if len(all) != 2 {
		t.Fatalf("unfiltered list should hold both runs, got %d", len(all))
	}
	roots, err := d.ListRootFlowRuns(ctx, "")
	if err != nil {
		t.Fatalf("list roots: %v", err)
	}
	if len(roots) != 1 || roots[0].ID != root.ID {
		t.Fatalf("root-only list should hold just the root, got %+v", roots)
	}
	// The child is hidden from the list, not gone.
	if _, err := d.GetFlowRun(ctx, child.ID); err != nil {
		t.Errorf("child should still be fetchable by id: %v", err)
	}
}

// TestListFlowRunTree_EmptyRootID verifies a missing root id yields nothing
// rather than scanning every run in the store.
func TestListFlowRunTree_EmptyRootID(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()
	if _, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW1"}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	tree, err := d.ListFlowRunTree(ctx, "")
	if err != nil {
		t.Fatalf("list tree: %v", err)
	}
	if len(tree) != 0 {
		t.Errorf("empty root id should yield no runs, got %d", len(tree))
	}
}
