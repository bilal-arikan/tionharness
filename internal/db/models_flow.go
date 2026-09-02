package db

import "encoding/json"

const FlowRunStateDeltaVersion = 1

// FlowRunStateDelta is one ordered mutation relative to the immutable legacy
// FlowRun.State checkpoint. CheckpointID is the SHA-256 digest of that state.
type FlowRunStateDelta struct {
	Version       int                        `json:"version"`
	CheckpointID  string                     `json:"checkpointId"`
	Sequence      uint64                     `json:"sequence"`
	Scalars       map[string]json.RawMessage `json:"scalars,omitempty"`
	OutputsUpsert map[string]string          `json:"outputsUpsert,omitempty"`
	OutputsDelete []string                   `json:"outputsDelete,omitempty"`
	TraceAppend   []json.RawMessage          `json:"traceAppend,omitempty"`
	ThreadAppend  []json.RawMessage          `json:"threadAppend,omitempty"`
	Spawned       json.RawMessage            `json:"spawned,omitempty"`
}

// Flow run statuses.
const (
	FlowRunning = "running"
	FlowSuccess = "success"
	FlowFailure = "failure"
	// FlowWaiting: the run paused at an await-input node and is durably suspended
	// until external input arrives (see ResumeWaitingFlow). Unlike "running", a
	// waiting run is NOT auto-resumed on boot — it sleeps until input, so it is
	// never an orphan.
	FlowWaiting = "waiting"
)

// Flow is a reusable multi-agent orchestration protocol. Graph holds the JSON
// node graph (see internal/orchestration.Graph).
type Flow struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Graph string `json:"graph"` // JSON
	// Emoji is an optional cosmetic glyph shown wherever the flow is presented or
	// picked (flow list, title bar, schedule/automation flow badges, run views).
	// Persisted independently via SetFlowEmoji so it survives graph/name saves.
	Emoji string `json:"emoji,omitempty"`
	// Tags are free-form labels on the flow, editable by both the user (UI) and
	// agents (set_flow_tags). Organizational only (they do not drive automations —
	// only session tags do).
	Tags []string `json:"tags,omitempty"`
	// Seed, when non-empty, marks this flow as a shipped built-in default
	// provisioned by EnsureDefaultFlows. The value is the stable seed key; it
	// lets seeding skip an already-present default and lets the UI recognize a
	// default. User- and agent-created flows leave it "".
	Seed string `json:"seed,omitempty"`
	// CreatedBy is the ID of the agent that created this flow via a
	// self-management tool ("" = created by the user). Agents may only
	// edit/delete agent-created flows.
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// FlowRun is one execution instance of a flow. State is the legacy-compatible
// restart checkpoint; DB readers materialize its ordered delta sidecar.
type FlowRun struct {
	ID     string `json:"id"`
	FlowID string `json:"flowId"`
	// SessionID links this run to the per-run transcript session it produced
	// (Session.Kind "flow"), so the chat view can resolve a flow session back to
	// the exact run — and its REAL graph/layout — instead of reifying the
	// transcript into a synthetic linear chain. Empty on pre-link runs.
	SessionID   string `json:"sessionId,omitempty"`
	DispatchKey string `json:"dispatchKey,omitempty"`
	// ParentRunID is the run that launched this one — a subflow/spawn node, or an
	// agent node's run_flow tool call. Empty means this is a ROOT run (started by
	// a user, schedule or automation). Child runs stay first-class: they get their
	// own row, status and viewer, and can also be launched standalone (in which
	// case they are roots themselves).
	ParentRunID string `json:"parentRunId,omitempty"`
	// ParentNodeID is the node in the PARENT's graph that launched this run. Needed
	// to attribute a child to the right node when one graph has several subflow or
	// spawn nodes. Empty on root runs (and on children launched by run_flow from an
	// agent node, where the node id is the agent node's).
	ParentNodeID string `json:"parentNodeId,omitempty"`
	// RootRunID is the top of this run's tree, so the whole tree is one query
	// instead of a level-by-level walk of ParentRunID. EMPTY MEANS SELF (this run
	// is the root) — never write a self-reference here, so a root needs no second
	// write after its id is generated. Use RootOf to read it.
	RootRunID string `json:"rootRunId,omitempty"`
	Status    string `json:"status"`
	Input     string `json:"input"`
	State     string `json:"state"` // JSON
	Output    string `json:"output"`
	Error     string `json:"error"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// RootOf returns the id of the top of this run's tree, resolving the "empty
// means self" encoding of RootRunID. A root run reports its own id.
func (r FlowRun) RootOf() string {
	if r.RootRunID != "" {
		return r.RootRunID
	}
	return r.ID
}

// IsRootRun reports whether this run is the top of its tree (nothing launched it).
func (r FlowRun) IsRootRun() bool { return r.ParentRunID == "" }
