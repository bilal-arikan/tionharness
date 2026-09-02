package db

import (
	"fmt"
	"strings"
)

// Trajectory ("Rota") is the declared-plus-observed graph of one unit of work:
// the phases a root session announced up front (from a coordinator recipe or the
// agent's own plan) and the sessions, flow runs and automation fires that then
// actually happened under them. It is a PROJECTION-FIRST entity: nothing
// executes a trajectory, runtime observers append to it and the UI/agent read
// it. One trajectory per root session, stored as that session's
// trajectory.json sidecar and summarised in trajectories/index.json for
// listing (see store_trajectory.go, _Docs/77 R4).
type Trajectory struct {
	ID            string `json:"id"`            // RTA<n>
	RootSessionID string `json:"rootSessionId"` // owning root session (same root as the coordinator tree)
	// TemplateRef names the recipe this trajectory was seeded from, as
	// "<slug>@<version>" ("" = agent-planned, no recipe).
	TemplateRef string `json:"templateRef,omitempty"`
	// Revision increases on every mutation. Readers echo it back on writes
	// (UpdateTrajectory expectedRev) so a stale UI or agent edit is refused
	// instead of clobbering a newer graph.
	Revision uint64           `json:"revision"`
	Status   string           `json:"status"` // TrajStatus*
	Nodes    []TrajectoryNode `json:"nodes"`
	Edges    []TrajectoryEdge `json:"edges"`
	// Meta is small free-form data owned by the projection layer (e.g. the lane
	// counter). Kept opaque so the store never needs to know about layout.
	Meta      map[string]string `json:"meta,omitempty"`
	CreatedAt int64             `json:"createdAt"`
	UpdatedAt int64             `json:"updatedAt"`
}

// Trajectory statuses.
const (
	TrajStatusPlanned   = "planned"   // seeded, no phase active yet
	TrajStatusRunning   = "running"   // at least one phase active
	TrajStatusWaiting   = "waiting"   // blocked on a gate / durable ask / await-input
	TrajStatusDone      = "done"      // terminal, all non-optional phases done
	TrajStatusFailed    = "failed"    // terminal, a phase failed without recovery
	TrajStatusAbandoned = "abandoned" // terminal, root session archived/deleted mid-way
)

// Trajectory node kinds — WHAT a node is.
const (
	TrajNodePhase      = "phase"      // a declared step (plan, code, review…)
	TrajNodeSession    = "session"    // a session bound under a phase (root, worker, spawn)
	TrajNodeAutomation = "automation" // an automation watcher: ghost until it fires
	TrajNodeFlowRun    = "flowrun"    // a flow run launched from inside the trajectory
	TrajNodeGate       = "gate"       // a human approval / verdict / schema check
	TrajNodeOptimizer  = "optimizer"  // the trajectory_end optimizer pass
)

// Trajectory node/edge origins — HOW a node is known.
const (
	TrajOriginDeclared = "declared" // announced by a recipe or the agent's plan
	TrajOriginObserved = "observed" // recorded by a runtime hook (spawn, fire, report…)
)

// Trajectory node states.
const (
	TrajStatePending = "pending"
	TrajStateActive  = "active"
	TrajStateDone    = "done"
	TrajStateFailed  = "failed"
	TrajStateSkipped = "skipped" // deliberately skipped, Reason set
	TrajStateGhost   = "ghost"   // declared automation/phase that has not (yet) happened
)

// Trajectory edge kinds.
const (
	TrajEdgeNext       = "next"        // phase order / handoff continuation
	TrajEdgeSpawned    = "spawned"     // a session spawned another
	TrajEdgeReported   = "reported"    // a worker reported back to its coordinator
	TrajEdgeFired      = "fired"       // an automation produced a session/run
	TrajEdgeFeeds      = "feeds"       // a phase's output is the next phase's input
	TrajEdgeBlockedBy  = "blocked_by"  // a gate holds a phase
	TrajEdgeForkedFrom = "forked_from" // a new ROOT session started from inside this one
)

// TrajectoryGate is the exit condition a phase must satisfy: an artifact with
// the given title, a validator verdict line, a human approval, or a schema check.
type TrajectoryGate struct {
	Kind  string `json:"kind"` // artifact | verdict | human | schema
	Value string `json:"value,omitempty"`
}

// TrajectoryNode is one vertex of the graph. RefKind/RefID bridge to the View
// layer (view.Ref{Kind, ID}) without importing it: "session"/SES…,
// "automation"/AUT…, "flowrun"/RUN….
type TrajectoryNode struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Origin  string `json:"origin"`
	Label   string `json:"label,omitempty"`
	RefKind string `json:"refKind,omitempty"`
	RefID   string `json:"refId,omitempty"`
	// PhaseID is the phase a session/automation/flowrun/gate node hangs under.
	PhaseID string `json:"phaseId,omitempty"`
	// Lane is the drawing row, assigned when the node is added and never changed
	// so a live graph does not reshuffle. 0 is the root session's lane.
	Lane    int    `json:"lane"`
	State   string `json:"state"`
	Profile string `json:"profile,omitempty"` // expected worker profile for a phase
	// Optional marks a declared phase the trajectory may finish without.
	Optional bool            `json:"optional,omitempty"`
	Gate     *TrajectoryGate `json:"gate,omitempty"`
	// Reason explains a skipped/failed/ghost state in one line.
	Reason  string `json:"reason,omitempty"`
	StartMs int64  `json:"startMs,omitempty"`
	EndMs   int64  `json:"endMs,omitempty"`
}

// TrajectoryEdge is one directed edge between two node ids.
type TrajectoryEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Origin string `json:"origin"`
}

// TrajectoryIndexEntry is the listing-sized summary of a trajectory kept in
// trajectories/index.json, so listing/filtering never opens the sidecars.
type TrajectoryIndexEntry struct {
	ID            string `json:"id"`
	RootSessionID string `json:"rootSessionId"`
	TemplateRef   string `json:"templateRef,omitempty"`
	Status        string `json:"status"`
	Revision      uint64 `json:"revision"`
	NodeCount     int    `json:"nodeCount"`
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
}

// IndexEntry projects the trajectory into its index row.
func (t Trajectory) IndexEntry() TrajectoryIndexEntry {
	return TrajectoryIndexEntry{
		ID: t.ID, RootSessionID: t.RootSessionID, TemplateRef: t.TemplateRef,
		Status: t.Status, Revision: t.Revision, NodeCount: len(t.Nodes),
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

var (
	trajStatuses    = []string{TrajStatusPlanned, TrajStatusRunning, TrajStatusWaiting, TrajStatusDone, TrajStatusFailed, TrajStatusAbandoned}
	trajNodeKinds   = []string{TrajNodePhase, TrajNodeSession, TrajNodeAutomation, TrajNodeFlowRun, TrajNodeGate, TrajNodeOptimizer}
	trajOrigins     = []string{TrajOriginDeclared, TrajOriginObserved}
	trajNodeStates  = []string{TrajStatePending, TrajStateActive, TrajStateDone, TrajStateFailed, TrajStateSkipped, TrajStateGhost}
	trajEdgeKinds   = []string{TrajEdgeNext, TrajEdgeSpawned, TrajEdgeReported, TrajEdgeFired, TrajEdgeFeeds, TrajEdgeBlockedBy, TrajEdgeForkedFrom}
	trajTerminalSet = map[string]bool{TrajStatusDone: true, TrajStatusFailed: true, TrajStatusAbandoned: true}
)

// IsTerminal reports whether the trajectory reached a final status.
func (t Trajectory) IsTerminal() bool { return trajTerminalSet[t.Status] }

// Node returns the node with the given id, ok=false when absent.
func (t Trajectory) Node(id string) (TrajectoryNode, bool) {
	for _, n := range t.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return TrajectoryNode{}, false
}

// Validate checks the graph's structural invariants: known enum values, unique
// node ids, edges between existing nodes, and every PhaseID pointing at a phase
// node. It does not judge semantics (which phases should exist) — that is the
// recipe schema's job.
func (t Trajectory) Validate() error {
	if strings.TrimSpace(t.RootSessionID) == "" {
		return fmt.Errorf("trajectory requires rootSessionId")
	}
	if !oneOf(t.Status, trajStatuses...) {
		return fmt.Errorf("invalid trajectory status %q", t.Status)
	}
	ids := make(map[string]string, len(t.Nodes))
	for i, n := range t.Nodes {
		if strings.TrimSpace(n.ID) == "" {
			return fmt.Errorf("node %d has an empty id", i)
		}
		if _, dup := ids[n.ID]; dup {
			return fmt.Errorf("duplicate node id %q", n.ID)
		}
		if !oneOf(n.Kind, trajNodeKinds...) {
			return fmt.Errorf("node %q: invalid kind %q", n.ID, n.Kind)
		}
		if !oneOf(n.Origin, trajOrigins...) {
			return fmt.Errorf("node %q: invalid origin %q", n.ID, n.Origin)
		}
		if !oneOf(n.State, trajNodeStates...) {
			return fmt.Errorf("node %q: invalid state %q", n.ID, n.State)
		}
		if n.Lane < 0 {
			return fmt.Errorf("node %q: negative lane", n.ID)
		}
		ids[n.ID] = n.Kind
	}
	for _, n := range t.Nodes {
		if n.PhaseID == "" {
			continue
		}
		kind, ok := ids[n.PhaseID]
		if !ok {
			return fmt.Errorf("node %q: phaseId %q is not a node", n.ID, n.PhaseID)
		}
		if kind != TrajNodePhase {
			return fmt.Errorf("node %q: phaseId %q is a %s, not a phase", n.ID, n.PhaseID, kind)
		}
	}
	for i, e := range t.Edges {
		if _, ok := ids[e.From]; !ok {
			return fmt.Errorf("edge %d: from %q is not a node", i, e.From)
		}
		if _, ok := ids[e.To]; !ok {
			return fmt.Errorf("edge %d: to %q is not a node", i, e.To)
		}
		if !oneOf(e.Kind, trajEdgeKinds...) {
			return fmt.Errorf("edge %d (%s→%s): invalid kind %q", i, e.From, e.To, e.Kind)
		}
		if !oneOf(e.Origin, trajOrigins...) {
			return fmt.Errorf("edge %d (%s→%s): invalid origin %q", i, e.From, e.To, e.Origin)
		}
	}
	return nil
}
