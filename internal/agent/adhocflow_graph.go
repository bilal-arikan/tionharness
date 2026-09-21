package agent

import (
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// Ids of the nodes the compiler generates around the caller's steps. The tool
// refuses step ids with the "__" prefix, so these can never collide.
const adhocStartNodeID = "__start"

// adhocRouteNodeID is the engine branch node behind a branch step. The step's own
// id is the transform node in front of it (see compileAdhocFlow), so the step id
// is what enters the trace and what incoming edges point at.
func adhocRouteNodeID(stepID string) string { return "__route_" + stepID }

// compileAdhocFlow turns a validated run_adhoc_flow plan into an orchestration
// graph. Every step keeps its id as a node id, so {{node.<step id>}} and the
// trace both speak the caller's vocabulary:
//
//   - parallel → ONE fan-out agent node (Node.Legs), Next = the step's next.
//   - branch   → a transform node (the step id) that loads the "on" step's output
//     into {{last}}, followed by an engine branch node that routes on it.
//   - end      → an end node.
//
// A start node is prepended as the required entry. The graph is not accumulate-
// mode: every leg runs in its own subagent session.
func compileAdhocFlow(spec tools.AdhocFlowSpec) (orchestration.Graph, error) {
	if len(spec.Steps) == 0 {
		return orchestration.Graph{}, fmt.Errorf("ad-hoc flow has no steps")
	}
	g := orchestration.Graph{Start: adhocStartNodeID}
	g.Nodes = append(g.Nodes, orchestration.Node{
		ID: adhocStartNodeID, Type: orchestration.NodeStart, Next: spec.Steps[0].ID,
	})
	for _, s := range spec.Steps {
		switch s.Type {
		case tools.AdhocStepParallel:
			legs := make([]orchestration.Leg, len(s.Tasks))
			for i, t := range s.Tasks {
				legs[i] = orchestration.Leg{
					Target:       t.Target,
					Task:         t.Task,
					Context:      t.Context,
					Model:        t.Model,
					Objective:    t.Objective,
					OutputFormat: t.OutputFormat,
					Boundaries:   t.Boundaries,
				}
			}
			g.Nodes = append(g.Nodes, orchestration.Node{
				ID: s.ID, Type: orchestration.NodeAgent, Title: s.ID, Legs: legs, Next: s.Next,
			})

		case tools.AdhocStepBranch:
			route := adhocRouteNodeID(s.ID)
			g.Nodes = append(g.Nodes, orchestration.Node{
				ID: s.ID, Type: orchestration.NodeTransform, Title: s.ID,
				Template: "{{node." + s.On + "}}", Next: route,
			})
			arms := make([]orchestration.Branch, len(s.Branches))
			for i, b := range s.Branches {
				arms[i] = orchestration.Branch{Contains: b.Value, Next: b.Next}
			}
			g.Nodes = append(g.Nodes, orchestration.Node{
				ID: route, Type: orchestration.NodeBranch, Title: s.ID,
				Branches: arms, MatchMode: s.MatchMode, JSONField: s.JSONField,
			})

		case tools.AdhocStepEnd:
			g.Nodes = append(g.Nodes, orchestration.Node{ID: s.ID, Type: orchestration.NodeEnd, Title: s.ID})

		default:
			return orchestration.Graph{}, fmt.Errorf("step %q has unsupported type %q", s.ID, s.Type)
		}
	}
	return g, nil
}
