package tools

import "context"

// run_adhoc_flow step types (v1). A parallel step fans out subagent legs, a
// branch step routes on an earlier parallel step's output, an end step stops.
const (
	AdhocStepParallel = "parallel"
	AdhocStepBranch   = "branch"
	AdhocStepEnd      = "end"
)

// run_adhoc_flow result labels. They are NOT a state machine of their own: the
// runtime derives them from the backing FlowRun's status and the engine trace —
// "skipped" means the graph never entered the step, "cancelled" means the run
// ended because the caller's turn was cancelled.
const (
	AdhocStatusDone      = "done"
	AdhocStatusFailed    = "failed"
	AdhocStatusSkipped   = "skipped"
	AdhocStatusCancelled = "cancelled"
)

// AdhocFlowSpec is a parsed, validated run_adhoc_flow request. Steps[0] is the
// entry. MaxRounds caps how many parallel-step executions the run may perform
// (0 = the runtime default); the runtime enforces the ceiling.
type AdhocFlowSpec struct {
	Steps     []AdhocFlowStep
	MaxRounds int
}

// AdhocFlowStep is one step of an ad-hoc plan. Fields apply per Type.
type AdhocFlowStep struct {
	ID   string
	Type string

	// parallel: the legs (same shape and validation as run_subagent's "tasks")
	// and the step that follows ("" = end of the plan).
	Tasks []RunAgentTask
	Next  string

	// branch: On names the parallel step whose output is matched; JSONField (when
	// set) matches that output's top-level JSON field instead of the raw text.
	// MatchMode is "equals" or "contains", uniform across the step's arms.
	On        string
	JSONField string
	MatchMode string
	Branches  []AdhocFlowBranch
}

// AdhocFlowBranch is one routing arm of a branch step. An empty Value is the
// default arm, taken when no other arm matches.
type AdhocFlowBranch struct {
	Value string
	Next  string
}

// AdhocFlowResult is the outcome of one run_adhoc_flow call. RunID is the real
// FlowRun id, so the caller can inspect it with get_view kind=flowrun.
type AdhocFlowResult struct {
	RunID  string                `json:"run_id"`
	Status string                `json:"status"`
	Steps  []AdhocFlowStepResult `json:"steps"`
	Final  string                `json:"final"`
	Error  string                `json:"error,omitempty"`
}

// AdhocFlowStepResult reports one plan step at node granularity: a parallel
// step is ONE entry whose Output summarizes all of its legs.
type AdhocFlowStepResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output string `json:"output"`
}

// RunAdhocFlowFunc executes an ad-hoc plan. It is implemented in the agent
// package (which owns the orchestration engine, the store and the delegation
// guards) and injected via context so the built-in tool — which must not import
// the agent package — can start a run. It returns a plain error when the plan is
// refused before a run exists; a run that fails is reported in the result.
type RunAdhocFlowFunc func(ctx context.Context, spec AdhocFlowSpec) (AdhocFlowResult, error)

// runAdhocFlowKey keys the RunAdhocFlowFunc on a request context.
type runAdhocFlowKey struct{}

// WithRunAdhocFlow attaches an ad-hoc flow runner to ctx so the run_adhoc_flow
// tool can start a run for this turn. Mirrors WithRunAgent.
func WithRunAdhocFlow(ctx context.Context, fn RunAdhocFlowFunc) context.Context {
	return context.WithValue(ctx, runAdhocFlowKey{}, fn)
}

// RunAdhocFlowFrom returns the runner attached to ctx, or nil when ad-hoc flows
// are not wired (e.g. provider-driven CLI loops).
func RunAdhocFlowFrom(ctx context.Context) RunAdhocFlowFunc {
	fn, _ := ctx.Value(runAdhocFlowKey{}).(RunAdhocFlowFunc)
	return fn
}
