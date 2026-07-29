package agent

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
)

// emitAndCapture publishes one flow-node frame for `run` and returns the event
// the bus delivered, so a test can assert on how the frame was tagged.
func emitAndCapture(t *testing.T, run db.FlowRun) events.Event {
	t.Helper()
	r := lifecycleRuntime(t)
	r.bus = events.NewBus()
	id, ch := r.bus.Subscribe()
	defer r.bus.Unsubscribe(id)

	r.emitFlowNode(run, orchestration.NodeEvent{Phase: "done", NodeID: "n1"})
	select {
	case e := <-ch:
		return e
	default:
		t.Fatal("no flow_node event was published")
		return events.Event{}
	}
}

// TestEmitFlowNode_TagsRootRunID verifies a child run's frame carries BOTH its
// own run id and the tree root. Without the root tag a viewer watching a composed
// run cannot receive frames from children, whose run ids it cannot know up front
// (they are created mid-run).
func TestEmitFlowNode_TagsRootRunID(t *testing.T) {
	e := emitAndCapture(t, db.FlowRun{
		ID: "FRN-child", FlowID: "FLW1",
		ParentRunID: "FRN-root", ParentNodeID: "sub1", RootRunID: "FRN-root",
	})
	if e.Type != "flow_node" {
		t.Fatalf("event type = %q, want flow_node", e.Type)
	}
	if got := e.Target["flowRunId"]; got != "FRN-child" {
		t.Errorf("flowRunId = %q, want the emitting run", got)
	}
	if got := e.Target["rootRunId"]; got != "FRN-root" {
		t.Errorf("rootRunId = %q, want the tree root", got)
	}
	if got := e.Target["flowId"]; got != "FLW1" {
		t.Errorf("flowId = %q, want FLW1", got)
	}
	// The parent's canvas needs to know WHICH run and which of its nodes this
	// child hangs off, or it cannot paint the child's progress without first
	// fetching the tree. The run id is not redundant: node ids are unique only
	// within one graph, so "sub1" alone could address two runs in the same tree.
	if got := e.Target["parentRunId"]; got != "FRN-root" {
		t.Errorf("parentRunId = %q, want the launching run", got)
	}
	if got := e.Target["parentNodeId"]; got != "sub1" {
		t.Errorf("parentNodeId = %q, want the launching node", got)
	}
}

// TestEmitFlowNode_RootRunTagsItself verifies a root run reports itself as the
// tree root, so a per-run subscriber and a tree subscriber resolve to the same
// key and neither has to special-case "this run has no parent".
func TestEmitFlowNode_RootRunTagsItself(t *testing.T) {
	// A root run carries no lineage fields at all — RootOf() resolves the
	// "empty means self" encoding, so the tag must still come out as its own id.
	e := emitAndCapture(t, db.FlowRun{ID: "FRN-root", FlowID: "FLW1"})
	if e.Target["flowRunId"] != e.Target["rootRunId"] {
		t.Errorf("a root run should tag itself as root, got run=%q root=%q",
			e.Target["flowRunId"], e.Target["rootRunId"])
	}
	// Nothing launched it, so there is neither a run nor a node to hang it off.
	if got := e.Target["parentRunId"]; got != "" {
		t.Errorf("parentRunId = %q, want empty for a root run", got)
	}
	if got := e.Target["parentNodeId"]; got != "" {
		t.Errorf("parentNodeId = %q, want empty for a root run", got)
	}
}
