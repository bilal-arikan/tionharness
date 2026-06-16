// Package orchestration runs structured multi-agent protocols described as a
// node graph. It supports sequential agent steps, conditional branching, and
// parallel fan-out + join, with restart-safe run state persisted by the caller.
package orchestration

import (
	"encoding/json"
	"fmt"
)

// Node types.
const (
	NodeAgent    = "agent"    // run an agent with a (templated) prompt, then go to Next
	NodeBranch   = "branch"   // route to a branch based on the last output
	NodeParallel = "parallel" // run several agent nodes concurrently, then JoinNext
)

// Graph is a reusable orchestration protocol.
type Graph struct {
	Start string `json:"start"` // entry node id
	Nodes []Node `json:"nodes"`
}

// Node is a single step. Fields are interpreted per Type.
type Node struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`

	// agent
	AgentID string `json:"agentId,omitempty"`
	Prompt  string `json:"prompt,omitempty"` // template: {{input}}, {{last}}, {{node.<id>}}
	Next    string `json:"next,omitempty"`   // next node id ("" = end)

	// branch
	Branches []Branch `json:"branches,omitempty"`

	// parallel
	Parallel []string `json:"parallel,omitempty"` // agent node ids to run concurrently
	JoinNext string   `json:"joinNext,omitempty"` // node after the join ("" = end)
}

// Branch is one routing rule of a branch node. An empty Contains is the default
// (else) arm and should be listed last.
type Branch struct {
	Contains string `json:"contains"` // case-insensitive substring of the last output
	Next     string `json:"next"`     // node id to route to ("" = end)
}

// ParseGraph decodes a graph from JSON.
func ParseGraph(data string) (Graph, error) {
	var g Graph
	if data == "" {
		return g, fmt.Errorf("empty graph")
	}
	if err := json.Unmarshal([]byte(data), &g); err != nil {
		return g, fmt.Errorf("invalid graph JSON: %w", err)
	}
	return g, nil
}

// node returns the node with the given id, or false.
func (g Graph) node(id string) (Node, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

// Validate checks structural integrity: a start node, unique ids, valid
// references, and agent nodes with an agent assigned.
func (g Graph) Validate() error {
	if g.Start == "" {
		return fmt.Errorf("graph has no start node")
	}
	seen := map[string]bool{}
	for _, n := range g.Nodes {
		if n.ID == "" {
			return fmt.Errorf("node with empty id")
		}
		if seen[n.ID] {
			return fmt.Errorf("duplicate node id %q", n.ID)
		}
		seen[n.ID] = true
	}
	if _, ok := g.node(g.Start); !ok {
		return fmt.Errorf("start node %q not found", g.Start)
	}

	ref := func(id, ctx string) error {
		if id == "" {
			return nil // end
		}
		if _, ok := g.node(id); !ok {
			return fmt.Errorf("%s references unknown node %q", ctx, id)
		}
		return nil
	}

	for _, n := range g.Nodes {
		switch n.Type {
		case NodeAgent:
			if n.AgentID == "" {
				return fmt.Errorf("agent node %q has no agentId", n.ID)
			}
			if err := ref(n.Next, "node "+n.ID); err != nil {
				return err
			}
		case NodeBranch:
			if len(n.Branches) == 0 {
				return fmt.Errorf("branch node %q has no branches", n.ID)
			}
			for _, b := range n.Branches {
				if err := ref(b.Next, "branch in "+n.ID); err != nil {
					return err
				}
			}
		case NodeParallel:
			if len(n.Parallel) == 0 {
				return fmt.Errorf("parallel node %q has no children", n.ID)
			}
			for _, c := range n.Parallel {
				cn, ok := g.node(c)
				if !ok {
					return fmt.Errorf("parallel node %q references unknown child %q", n.ID, c)
				}
				if cn.Type != NodeAgent {
					return fmt.Errorf("parallel child %q must be an agent node", c)
				}
			}
			if err := ref(n.JoinNext, "join of "+n.ID); err != nil {
				return err
			}
		default:
			return fmt.Errorf("node %q has unknown type %q", n.ID, n.Type)
		}
	}
	return nil
}
