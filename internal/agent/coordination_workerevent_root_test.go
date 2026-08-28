package agent

import (
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// captureWorkerEvent runs emit and returns the single worker event the bus
// delivered, so a test can assert on how the transition was tagged.
func captureWorkerEvent(t *testing.T, rt *Runtime, emit func()) events.Event {
	t.Helper()
	id, ch := rt.bus.Subscribe()
	defer rt.bus.Unsubscribe(id)

	emit()
	select {
	case e := <-ch:
		return e
	default:
		t.Fatal("no worker event was published")
		return events.Event{}
	}
}

// TestWorkerEventTagsRootCoordinator verifies a depth-2 worker's transitions carry
// the tree ROOT alongside its direct sub-coordinator. The root coordinator's chat
// keys its running-worker banner on the root id; without this tag a grandchild
// starting or finishing never reaches it and the banner goes stale.
func TestWorkerEventTagsRootCoordinator(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.bus = events.NewBus()

	root := newTreeNode(t, rt, "root", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid", root.ID, root.ID, 1, true)
	leaf := newTreeNode(t, rt, "leaf", mid.ID, root.ID, 2, false)
	worker := db.Agent{Name: "leafAgent"}

	start := captureWorkerEvent(t, rt, func() {
		rt.emitWorkerStartEvent(worker, leaf.ID, mid.ID)
	})
	if got := start.Target["coordinatorId"]; got != mid.ID {
		t.Errorf("start coordinatorId = %q, want the direct coordinator %q", got, mid.ID)
	}
	if got := start.Target["rootCoordinatorId"]; got != root.ID {
		t.Errorf("start rootCoordinatorId = %q, want the tree root %q", got, root.ID)
	}

	done := captureWorkerEvent(t, rt, func() {
		rt.emitWorkerEvent(worker, leaf.ID, mid.ID, "completed")
	})
	if got := done.Target["coordinatorId"]; got != mid.ID {
		t.Errorf("completed coordinatorId = %q, want the direct coordinator %q", got, mid.ID)
	}
	if got := done.Target["rootCoordinatorId"]; got != root.ID {
		t.Errorf("completed rootCoordinatorId = %q, want the tree root %q", got, root.ID)
	}
}

// TestWorkerEventOmitsRootWhenCoordinatorIsRoot verifies a top-level worker's event
// carries no root tag: its coordinator already IS the root, and a duplicate id
// would only make the frontend publish the same bus key twice.
func TestWorkerEventOmitsRootWhenCoordinatorIsRoot(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.bus = events.NewBus()

	root := newTreeNode(t, rt, "root", "", "", 0, true)
	leaf := newTreeNode(t, rt, "leaf", root.ID, root.ID, 1, false)

	e := captureWorkerEvent(t, rt, func() {
		rt.emitWorkerEvent(db.Agent{Name: "leafAgent"}, leaf.ID, root.ID, "completed")
	})
	if got := e.Target["coordinatorId"]; got != root.ID {
		t.Errorf("coordinatorId = %q, want %q", got, root.ID)
	}
	if got, ok := e.Target["rootCoordinatorId"]; ok {
		t.Errorf("rootCoordinatorId = %q, want it omitted when the coordinator is the root", got)
	}
}
