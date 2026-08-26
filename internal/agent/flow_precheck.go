package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/skills"
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
// ValidateFlowGraph runs the full structural + semantic check (Graph.Validate
// followed by validateFlowPreconditions) on a graph without creating a FlowRun.
// Flow save paths (API create/update handlers) call this so a graph that
// references a missing or unrunnable agent is rejected at save time instead of
// only surfacing once the flow is run.
func (r *Runtime) ValidateFlowGraph(ctx context.Context, g orchestration.Graph) error {
	if err := validateFlowReferences(g); err != nil {
		return err
	}
	if err := g.Validate(); err != nil {
		return err
	}
	return r.validateFlowPreconditions(ctx, g)
}

// validateFlowReferences collects graph identity and node-reference faults before
// Graph.Validate's broader, first-error structural validation. Keeping this pass
// first also prevents provider builds for a graph that cannot be executed.
func validateFlowReferences(g orchestration.Graph) error {
	var problems []error
	nodes := make(map[string]struct{}, len(g.Nodes))
	for _, n := range g.Nodes {
		if n.ID == "" {
			problems = append(problems, fmt.Errorf("node has an empty id"))
			continue
		}
		if _, exists := nodes[n.ID]; exists {
			problems = append(problems, fmt.Errorf("duplicate node id %q", n.ID))
		}
		nodes[n.ID] = struct{}{}
	}

	check := func(nodeID, field, target string) {
		if target == "" {
			return // Empty optional transitions mean end; required fields are checked by Graph.Validate.
		}
		if _, exists := nodes[target]; !exists {
			problems = append(problems, fmt.Errorf("node %q field %s references unknown node %q", nodeID, field, target))
		}
	}

	if g.Start == "" {
		problems = append(problems, fmt.Errorf("graph has no start node"))
	} else {
		check("graph", "start", g.Start)
	}
	for _, n := range g.Nodes {
		check(n.ID, "next", n.Next)
		check(n.ID, "joinNext", n.JoinNext)
		check(n.ID, "body", n.Body)
		check(n.ID, "loopNext", n.LoopNext)
		check(n.ID, "spawnRef", n.SpawnRef)
		for i, target := range n.Parallel {
			check(n.ID, fmt.Sprintf("parallel[%d]", i), target)
		}
		for i, branch := range n.Branches {
			check(n.ID, fmt.Sprintf("branches[%d].next", i), branch.Next)
		}
	}

	return errors.Join(problems...)
}

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
		// A coordinator node runs its agent exactly like an agent node does (through
		// a session turn), so it needs the same "agent exists + provider builds" check.
		case orchestration.NodeAgent, orchestration.NodeCoordinator:
			c, done := checked[n.AgentID]
			if !done {
				c = agentCheck{err: r.checkFlowAgent(ctx, n.AgentID)}
				checked[n.AgentID] = c
			}
			if c.err != nil {
				problems = append(problems, fmt.Errorf("node %q: %w", n.ID, c.err))
			}
			// A coordination recipe that was renamed/deleted since the flow was drawn
			// would otherwise only surface once the node ran — and a silent fallback to
			// free coordination is exactly the wrong recovery (the author picked the
			// recipe on purpose).
			if n.Type == orchestration.NodeCoordinator && strings.TrimSpace(n.Workflow) != "" {
				if _, err := skills.ResolveCoordinatorWorkflow(r.skills, n.Workflow); err != nil {
					problems = append(problems, fmt.Errorf("node %q: %w", n.ID, err))
				}
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
		return fmt.Errorf("agentId %q must be an existing agent ID (not a node id or agent name): %w", agentID, err)
	}
	// The agent exists but its provider may have been deleted or left
	// unconfigured (missing API key) since the flow was authored. Registry.Get
	// covers both — this is exactly the failure the engine would otherwise hit on
	// the node's first completion call.
	if _, err := r.providers.Get(agent.ProviderRef()); err != nil {
		return fmt.Errorf("agent %q (%s) cannot run: %w", agent.Name, agentID, err)
	}
	return nil
}
