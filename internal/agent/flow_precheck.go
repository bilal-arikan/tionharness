package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/orchestration"
)

// validateFlowPreconditions is the semantic (DB- and registry-backed) half of
// flow validation, run right after the structural orchestration.Graph.Validate()
// and *before* a FlowRun row is created.
//
// Graph.Validate() only proves the graph is well-formed on its own terms: a
// start node exists, ids are unique, every Next/Branch/Parallel reference
// resolves, agent nodes carry an agentId. It cannot know whether that agentId
// still exists, whether the agent's provider is configured, or whether a
// transform/delay node carries usable parameters — orchestration must not
// depend on db or providers.
//
// Those gaps only surfaced mid-run: the user triggered a flow, a FlowRun row was
// created, and the engine failed on the first offending node. This check moves
// them to the front so an unrunnable flow is rejected with no run record at all.
//
// Every problem in the graph is reported, not just the first: joined into one
// error so the user fixes the flow in a single pass instead of rediscovering the
// next fault on every retry.
func (r *Runtime) validateFlowPreconditions(ctx context.Context, g orchestration.Graph) error {
	var problems []error

	// Agents are looked up once per distinct id: a graph may reference the same
	// agent from many nodes, and a fan-out flow would otherwise re-read it per node.
	type agentCheck struct {
		err error // nil once the agent resolved and its provider built
	}
	checked := map[string]agentCheck{}

	for _, n := range g.Nodes {
		switch n.Type {
		case orchestration.NodeAgent:
			c, done := checked[n.AgentID]
			if !done {
				c = agentCheck{err: r.checkFlowAgent(ctx, n.AgentID)}
				checked[n.AgentID] = c
			}
			if c.err != nil {
				problems = append(problems, fmt.Errorf("node %q: %w", n.ID, c.err))
			}

		case orchestration.NodeTransform:
			// A transform node's only job is to render its template as the node's
			// output. An empty template silently produces an empty output that the
			// downstream {{last}} then carries forward — a misconfiguration, not a
			// valid no-op.
			if strings.TrimSpace(n.Template) == "" {
				problems = append(problems, fmt.Errorf("node %q: transform node has an empty template", n.ID))
			}

		case orchestration.NodeDelay:
			// The engine sleeps DelayMs; a negative value is never intentional.
			if n.DelayMs < 0 {
				problems = append(problems, fmt.Errorf("node %q: delay node has a negative delayMs (%d)", n.ID, n.DelayMs))
			}
		}
	}

	return errors.Join(problems...)
}

// checkFlowAgent resolves one agent node's agent and proves its provider can be
// built. Returns nil when the node is runnable.
func (r *Runtime) checkFlowAgent(ctx context.Context, agentID string) error {
	agent, err := r.db.GetAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("references agent %q which no longer exists: %w", agentID, err)
	}
	// The agent exists but its provider may have been deleted or left
	// unconfigured (missing API key) since the flow was authored. Registry.Get
	// covers both — this is exactly the failure the engine would otherwise hit on
	// the node's first completion call.
	if _, err := r.providers.Get(agent.Provider); err != nil {
		return fmt.Errorf("agent %q (%s) cannot run: %w", agent.Name, agentID, err)
	}
	return nil
}
