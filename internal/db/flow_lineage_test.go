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

// TestListFlowRunTree_ParentPrecedesChildren verifies a grandchild several levels
// down still resolves via RootRunID (one scan, not a per-level walk) and that a
// parent always precedes its children.
//
// Ordering may NOT lean on CreatedAt: it is second-granular, so these runs — all
// created in the same instant, exactly like a real composed flow — carry
// identical timestamps. An implementation that sorted by time alone returned them
// in arbitrary order.
func TestListFlowRunTree_ParentPrecedesChildren(t *testing.T) {
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
		t.Errorf("tree order wrong: %s, %s, %s (want %s, %s, %s)",
			tree[0].ID, tree[1].ID, tree[2].ID, root.ID, child.ID, grand.ID)
	}
	for _, r := range tree {
		if r.ID == other.ID {
			t.Error("an unrelated run leaked into the tree")
		}
	}
}

// TestListFlowRunTree_GroupsSiblingsBreadthFirst verifies a fan-out (spawn) tree
// comes back breadth-first: both children of the root before either grandchild,
// so a renderer can indent by depth without reordering.
func TestListFlowRunTree_GroupsSiblingsBreadthFirst(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	root, _ := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW-root"})
	a, _ := d.CreateFlowRun(ctx, FlowRun{
		FlowID: "FLW-a", ParentRunID: root.ID, ParentNodeID: "fan", RootRunID: root.ID,
	})
	b, _ := d.CreateFlowRun(ctx, FlowRun{
		FlowID: "FLW-b", ParentRunID: root.ID, ParentNodeID: "fan", RootRunID: root.ID,
	})
	// A grandchild under the FIRST sibling: breadth-first must still place it
	// after the second sibling.
	aa, _ := d.CreateFlowRun(ctx, FlowRun{
		FlowID: "FLW-aa", ParentRunID: a.ID, ParentNodeID: "sub", RootRunID: root.ID,
	})

	tree, err := d.ListFlowRunTree(ctx, root.ID)
	if err != nil {
		t.Fatalf("list tree: %v", err)
	}
	got := []string{tree[0].ID, tree[1].ID, tree[2].ID, tree[3].ID}
	want := []string{root.ID, a.ID, b.ID, aa.ID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("breadth-first order wrong: got %v, want %v", got, want)
		}
	}
}

// TestListFlowRunTree_KeepsUnreachableMembers verifies a run whose parent row is
// gone is still returned (appended at the end) rather than silently vanishing —
// a missing intermediate must not hide work that actually ran.
func TestListFlowRunTree_KeepsUnreachableMembers(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()

	root, _ := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW-root"})
	orphan, _ := d.CreateFlowRun(ctx, FlowRun{
		FlowID: "FLW-orphan", ParentRunID: "RUN-deleted", RootRunID: root.ID,
	})

	tree, err := d.ListFlowRunTree(ctx, root.ID)
	if err != nil {
		t.Fatalf("list tree: %v", err)
	}
	if len(tree) != 2 || tree[0].ID != root.ID || tree[1].ID != orphan.ID {
		t.Fatalf("expected root then the unreachable member, got %+v", tree)
	}
}

// TestFlowRunSeq_OrdersBeyondTen locks the tie-break that makes same-second
// ordering deterministic: ids are not zero-padded, so a lexicographic compare
// would place "RUN10" before "RUN2".
func TestFlowRunSeq_OrdersBeyondTen(t *testing.T) {
	if !(flowRunSeq("RUN2") < flowRunSeq("RUN10")) {
		t.Error("RUN2 must sort before RUN10")
	}
	if flowRunSeq("RUN") != -1 || flowRunSeq("") != -1 {
		t.Error("an id with no counter should report -1")
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
