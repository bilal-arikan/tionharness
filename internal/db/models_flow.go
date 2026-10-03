package db

import "encoding/json"

// Flow run statuses.
const (
	FlowRunning = "running"
	FlowSuccess = "success"
	FlowFailure = "failure"
)

// Flow evolution policies: who may change a flow on its own.
const (
	FlowPolicyOff     = "off"     // the observer never runs
	FlowPolicyPropose = "propose" // the observer files proposals; a human applies them
	FlowPolicyAuto    = "auto"    // the observer applies confident proposals itself
)

// Who authored a flow version / prompt version / proposal.
const (
	FlowAuthorUser     = "user"
	FlowAuthorAgent    = "agent"
	FlowAuthorObserver = "observer"
	FlowAuthorSystem   = "system"
)

// Proposal statuses.
const (
	ProposalPending  = "pending"
	ProposalApplied  = "applied"
	ProposalRejected = "rejected"
	ProposalInvalid  = "invalid"
)

// FlowAuthor records who made a change: the kind plus the agent id when an
// agent or the observer did it.
type FlowAuthor struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}

// FlowPolicy configures the observer (the end-of-run optimizer) for one flow.
type FlowPolicy struct {
	Mode string `json:"mode"` // off | propose | auto
	// EveryRuns is how many new runs accumulate before the observer looks again.
	EveryRuns int `json:"everyRuns"`
	// MinConfidence gates auto-apply: a proposal below it is filed, not applied.
	MinConfidence float64 `json:"minConfidence"`
	// MaxNodes is the growth budget the observer and agents may not exceed.
	MaxNodes int `json:"maxNodes"`
	// AllowPromptChanges lets the observer also propose soul/identity edits.
	AllowPromptChanges bool `json:"allowPromptChanges"`
}

// DefaultFlowPolicy is what a new flow gets.
func DefaultFlowPolicy() FlowPolicy {
	return FlowPolicy{Mode: FlowPolicyPropose, EveryRuns: 5, MinConfidence: 0.7, MaxNodes: 16, AllowPromptChanges: true}
}

// Normalized fills zero fields with the defaults.
func (p FlowPolicy) Normalized() FlowPolicy {
	d := DefaultFlowPolicy()
	switch p.Mode {
	case FlowPolicyOff, FlowPolicyPropose, FlowPolicyAuto:
	default:
		p.Mode = d.Mode
	}
	if p.EveryRuns <= 0 {
		p.EveryRuns = d.EveryRuns
	}
	if p.MinConfidence <= 0 || p.MinConfidence > 1 {
		p.MinConfidence = d.MinConfidence
	}
	if p.MaxNodes <= 0 {
		p.MaxNodes = d.MaxNodes
	}
	return p
}

// FlowStats is the rolling bookkeeping the store keeps per flow.
type FlowStats struct {
	Runs        int    `json:"runs"`
	Success     int    `json:"success"`
	Failure     int    `json:"failure"`
	LastRunAt   int64  `json:"lastRunAt,omitempty"`
	LastRunID   string `json:"lastRunId,omitempty"`
	TotalMs     int64  `json:"totalMs"`
	TotalTokens int64  `json:"totalTokens"`
	// RunsAtOptimize is Runs when the observer last looked; the policy's
	// EveryRuns counts from here.
	RunsAtOptimize int   `json:"runsAtOptimize"`
	LastOptimizeAt int64 `json:"lastOptimizeAt,omitempty"`
	// Graded / GradeSum keep the decision model's quality grades (1..5) so
	// the average is one division away (see FlowRun.Grade).
	Graded   int `json:"graded,omitempty"`
	GradeSum int `json:"gradeSum,omitempty"`
}

// AvgGrade is the mean grade of the graded runs (0 when none).
func (s FlowStats) AvgGrade() float64 {
	if s.Graded <= 0 {
		return 0
	}
	return float64(s.GradeSum) / float64(s.Graded)
}

// Flow is an agent's main flow: the head graph plus policy and stats. The
// version history lives beside it (FlowVersion rows); Graph always equals the
// head version's graph.
type Flow struct {
	ID      string     `json:"id"`
	AgentID string     `json:"agentId"`
	Name    string     `json:"name"`
	Graph   string     `json:"graph"` // JSON of flow.Graph (head)
	Version int        `json:"version"`
	Policy  FlowPolicy `json:"policy"`
	Stats   FlowStats  `json:"stats"`
	// Note is a free-form description agents and the observer may update to
	// explain what the flow is for.
	Note      string `json:"note,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// FlowVersion is one immutable snapshot of a flow's graph.
type FlowVersion struct {
	FlowID     string     `json:"flowId"`
	Version    int        `json:"version"`
	Parent     int        `json:"parent,omitempty"`
	Graph      string     `json:"graph"`
	Author     FlowAuthor `json:"author"`
	Reason     string     `json:"reason,omitempty"`
	ProposalID string     `json:"proposalId,omitempty"`
	// Diff summarizes what changed against Parent ("+critic, ~respond").
	Diff      string `json:"diff,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// FlowRunUsage is the token footprint of one run.
type FlowRunUsage struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	LLMCalls     int   `json:"llmCalls"`
}

// FlowRun is one execution of a flow — one agent turn.
type FlowRun struct {
	ID        string `json:"id"`
	FlowID    string `json:"flowId"`
	AgentID   string `json:"agentId"`
	SessionID string `json:"sessionId,omitempty"`
	// MessageID is the assistant message the run produced (set when known).
	MessageID string `json:"messageId,omitempty"`
	Version   int    `json:"version"`
	Trigger   string `json:"trigger,omitempty"` // call kind: chat | schedule | spawn | …
	Status    string `json:"status"`
	Input     string `json:"input"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
	// Steps is the node trace (flow.Step JSON array).
	Steps      json.RawMessage `json:"steps,omitempty"`
	StepCount  int             `json:"stepCount"`
	DurationMs int64           `json:"durationMs"`
	Usage      FlowRunUsage    `json:"usage"`
	// Feedback mirrors the user's rating of the produced message (+1 / -1 / 0).
	Feedback int `json:"feedback,omitempty"`
	// Grade is the decision model's quality grade of the reply (1..5, 0 =
	// not graded; the flow-grade authority) with the confidence behind it.
	Grade           int     `json:"grade,omitempty"`
	GradeConfidence float64 `json:"gradeConfidence,omitempty"`
	CreatedAt       int64   `json:"createdAt"`
	UpdatedAt       int64   `json:"updatedAt"`
}

// FlowPromptChange is the observer's optional edit of the agent's prompts.
type FlowPromptChange struct {
	Soul     *string `json:"soul,omitempty"`
	Identity *string `json:"identity,omitempty"`
}

// FlowProposal is one suggested change to a flow (and optionally its agent's
// prompts), filed by the observer or an agent. Ops is a flow.Op JSON array.
type FlowProposal struct {
	ID          string            `json:"id"`
	FlowID      string            `json:"flowId"`
	AgentID     string            `json:"agentId"`
	BaseVersion int               `json:"baseVersion"`
	Author      FlowAuthor        `json:"author"`
	Trigger     string            `json:"trigger,omitempty"` // auto | manual | agent
	Ops         json.RawMessage   `json:"ops,omitempty"`
	Prompt      *FlowPromptChange `json:"prompt,omitempty"`
	Reason      string            `json:"reason"`
	Expected    string            `json:"expected,omitempty"` // expected effect, in words
	Confidence  float64           `json:"confidence"`
	Evidence    string            `json:"evidence,omitempty"` // which runs it looked at
	Status      string            `json:"status"`
	// AppliedVersion is the flow version the proposal produced when applied.
	AppliedVersion int    `json:"appliedVersion,omitempty"`
	Error          string `json:"error,omitempty"` // why it is invalid / failed to apply
	CreatedAt      int64  `json:"createdAt"`
	ResolvedAt     int64  `json:"resolvedAt,omitempty"`
}

// AgentPromptVersion is one snapshot of an agent's soul + identity, kept so
// prompt evolution is as reversible as flow evolution.
type AgentPromptVersion struct {
	AgentID    string     `json:"agentId"`
	Version    int        `json:"version"`
	Soul       string     `json:"soul"`
	Identity   string     `json:"identity"`
	Author     FlowAuthor `json:"author"`
	Reason     string     `json:"reason,omitempty"`
	ProposalID string     `json:"proposalId,omitempty"`
	CreatedAt  int64      `json:"createdAt"`
}
