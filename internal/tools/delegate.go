package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/providers"
)

// DelegateResult is the outcome of a synchronous agent→agent delegation: which
// agent answered and the reply text it produced.
type DelegateResult struct {
	AgentName string
	Reply     string
}

// DelegateRunner runs a delegated sub-agent turn synchronously and returns its
// reply. It is implemented in the agent package (which owns the runtime, the
// agent store and the call-graph guards) and injected via context so built-in
// tools — which must not import the agent package — can summon another agent.
//
// target is the requested agent (display name or id); task is the instruction.
// The runner enforces all loop-protection (depth limit, cycle/visited-set,
// per-turn call budget) and returns a plain error when a guard refuses the call,
// so the model can adapt instead of the turn aborting.
type DelegateRunner func(ctx context.Context, target, task string) (DelegateResult, error)

// delegationKey keys the DelegateRunner on a request context.
type delegationKey struct{}

// WithDelegation attaches a delegation runner to ctx so the call_agent tool can
// summon another agent for this turn. Mirrors WithAsker: kept in the tools
// package so built-ins reach it without importing the agent package.
func WithDelegation(ctx context.Context, fn DelegateRunner) context.Context {
	return context.WithValue(ctx, delegationKey{}, fn)
}

// DelegationFrom returns the runner attached to ctx, or nil when delegation is
// not wired (autonomous-only paths, or the feature disabled).
func DelegationFrom(ctx context.Context) DelegateRunner {
	fn, _ := ctx.Value(delegationKey{}).(DelegateRunner)
	return fn
}

// callAgentInput is the argument shape for the call_agent tool.
type callAgentInput struct {
	Agent string `json:"agent"`
	Task  string `json:"task"`
}

// CallAgentTool lets one agent hand a sub-task to another agent in the same
// workspace and wait (synchronously) for its reply. The summoned agent inherits
// the current conversation context plus the task, runs its own full turn (its
// own persona, model and tools), and its answer is returned as this tool's
// result so the caller can fold it into its own response.
//
// All loop-protection lives in the injected runner: a call that would recurse
// too deep, revisit an agent already in the chain, or blow the per-turn call
// budget comes back as an error result the model can react to.
type CallAgentTool struct{}

// NewCallAgentTool constructs the call_agent tool.
func NewCallAgentTool() CallAgentTool { return CallAgentTool{} }

func (CallAgentTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "call_agent",
		Description: "Delegate a sub-task to another agent in this workspace and wait for its reply. " +
			"Use this to consult a specialist (e.g. a reviewer, a coder, a researcher) when the " +
			"work is outside your focus. The other agent sees the current conversation plus your " +
			"task, answers from its own perspective, and its reply is returned to you so you can " +
			"build on it. Keep delegation shallow: prefer answering yourself when you can.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "agent": { "type": "string", "description": "The name (or id) of the agent to delegate to." },
    "task": { "type": "string", "description": "A clear, self-contained instruction describing what you need from that agent." }
  },
  "required": ["agent", "task"],
  "additionalProperties": false
}`),
	}
}

func (CallAgentTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in callAgentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid call_agent input: %w", err)
	}
	in.Agent = strings.TrimSpace(in.Agent)
	in.Task = strings.TrimSpace(in.Task)
	if in.Agent == "" || in.Task == "" {
		return "", fmt.Errorf("both \"agent\" and \"task\" are required")
	}
	run := DelegationFrom(ctx)
	if run == nil {
		return "", fmt.Errorf("agent delegation is not available in this context")
	}
	res, err := run(ctx, in.Agent, in.Task)
	if err != nil {
		return "", err
	}
	// Surface the answer with a small header so the caller (and the trace card)
	// clearly attribute the reply to the summoned agent.
	header := "Reply from agent \"" + res.AgentName + "\":\n\n"
	return header + res.Reply, nil
}
