package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

// flowRunIDKey carries the active flow run's id through the engine into the
// AgentRunner, so a node's captured tool/thinking steps can be written to a
// sidecar keyed by (runID, nodeID). The engine tags the node id separately
// (orchestration.WithNodeID); together they address one node's steps.
type flowRunIDKey struct{}

// withFlowRunID tags ctx with the flow run id (set once per run in driveFlow).
func withFlowRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, flowRunIDKey{}, runID)
}

// flowRunIDFromContext returns the flow run id set by withFlowRunID ("" if none,
// e.g. a non-flow completion path).
func flowRunIDFromContext(ctx context.Context) string {
	s, _ := ctx.Value(flowRunIDKey{}).(string)
	return s
}

// flowRootIDKey carries the id of the TOP of the active run's tree alongside the
// run id, so a run launched from inside another one can record its RootRunID
// without reading its parent back from the store.
type flowRootIDKey struct{}

// withFlowRootID tags ctx with the run tree's root id (set once per run in
// driveFlow, next to withFlowRunID).
func withFlowRootID(ctx context.Context, rootID string) context.Context {
	return context.WithValue(ctx, flowRootIDKey{}, rootID)
}

// flowRootIDFromContext returns the run tree's root id ("" if none).
func flowRootIDFromContext(ctx context.Context) string {
	s, _ := ctx.Value(flowRootIDKey{}).(string)
	return s
}

// runLineage derives the parent/root linkage for a flow run about to be created
// under ctx. A ctx carrying a flow run id means we are INSIDE that run, so the
// new run is its child; otherwise this is a root run and all three are empty
// (RootRunID encodes "root" as empty — see db.FlowRun.RootRunID).
//
// This deliberately also catches a flow started by an agent node's run_flow tool:
// that call inherits the node's context, so the run it starts joins the tree
// instead of appearing as an unrelated root.
func runLineage(ctx context.Context) (parentRunID, parentNodeID, rootRunID string) {
	parentRunID = flowRunIDFromContext(ctx)
	if parentRunID == "" {
		return "", "", ""
	}
	rootRunID = flowRootIDFromContext(ctx)
	if rootRunID == "" {
		// Parent predates lineage tracking (or is itself a root): the tree tops out
		// at the parent.
		rootRunID = parentRunID
	}
	return parentRunID, orchestration.NodeIDFromContext(ctx), rootRunID
}

// flowRunStepsDir is where a run's per-node step sidecars live:
// <store>/flow_runs/<runID>/. Kept out of the entity store proper so it never
// bloats the restart-safe flow-run State (which is re-marshaled wholesale after
// every node — see WS5/TSK64).
func (r *Runtime) flowRunStepsDir(runID string) string {
	return filepath.Join(r.db.Root(), "flow_runs", runID)
}

// flowNodeStepsPath is the sidecar file for one node's steps within a run.
func (r *Runtime) flowNodeStepsPath(runID, nodeID string) string {
	return filepath.Join(r.flowRunStepsDir(runID), "steps-"+sanitizeNodeID(nodeID)+".json")
}

// sanitizeNodeID keeps a node id safe as a filename component. Node ids are
// author-controlled, so guard against path separators / traversal.
func sanitizeNodeID(id string) string {
	out := make([]rune, 0, len(id))
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "node"
	}
	return string(out)
}

// captureFlowNodeSteps writes the given trace to this node's sidecar when both
// the run id (ctx via withFlowRunID) and node id (ctx via orchestration.WithNodeID)
// are present. Best-effort: a failed write costs the node's step detail, never
// the run. No-op when there are no steps or the context lacks the ids (non-flow
// path), so it is safe to call unconditionally from the shared complete() paths.
func (r *Runtime) captureFlowNodeSteps(ctx context.Context, steps []TurnStep) {
	if len(steps) == 0 {
		return
	}
	runID := flowRunIDFromContext(ctx)
	nodeID := orchestration.NodeIDFromContext(ctx)
	if runID == "" || nodeID == "" {
		return
	}
	if err := r.writeFlowNodeSteps(runID, nodeID, steps); err != nil {
		r.logger.Warn("flow node steps sidecar write failed", "run", runID, "node", nodeID, "error", err)
	}
}

// emitFlowNodeStepCtx broadcasts one live tool/thinking step for the currently
// executing agent node, keyed by (runID, nodeID) from the context, so any window
// viewing the run renders the step the moment it happens (the flow counterpart of
// the chat's session_step). No-op off the flow path (missing ids). Best-effort:
// the sidecar written at node end is the durable record; a dropped live frame
// only costs a little latency.
func (r *Runtime) emitFlowNodeStepCtx(ctx context.Context, step TurnStep) {
	runID := flowRunIDFromContext(ctx)
	nodeID := orchestration.NodeIDFromContext(ctx)
	if runID == "" || nodeID == "" {
		return
	}
	// Shared emit seam with the chat session_step feed (see publishStep): same
	// marshal + bus envelope, distinct event type + (flowRunId, nodeId) target.
	r.publishStep("flow_node_step", map[string]string{"flowRunId": runID, "nodeId": nodeID}, step)
}

// writeFlowNodeSteps atomically persists one node's steps (tmp + rename), so a
// crash mid-write can't leave a half-written sidecar that breaks the reader.
func (r *Runtime) writeFlowNodeSteps(runID, nodeID string, steps []TurnStep) error {
	dir := r.flowRunStepsDir(runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(steps)
	if err != nil {
		return err
	}
	path := r.flowNodeStepsPath(runID, nodeID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadFlowNodeSteps loads one node's captured steps for the run inspector. A
// missing sidecar (node produced no steps, or a run from before this feature)
// returns (nil, nil) — an empty, non-error result the API surfaces as [].
func (r *Runtime) ReadFlowNodeSteps(runID, nodeID string) ([]TurnStep, error) {
	data, err := os.ReadFile(r.flowNodeStepsPath(runID, nodeID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var steps []TurnStep
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, err
	}
	return steps, nil
}
