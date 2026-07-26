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
	NodeAgent     = "agent"     // run an agent with a (templated) prompt, then go to Next
	NodeBranch    = "branch"    // route by matching the last output (see MatchMode)
	NodeParallel  = "parallel"  // run several agent nodes concurrently, then JoinNext
	NodeDelay     = "delay"     // wait DelayMs, then go to Next (no LLM)
	NodeTransform = "transform" // emit a rendered template as output, then Next (no LLM)
	NodeLoop      = "loop"      // repeat a body sub-chain until MaxIters/Until, then LoopNext
)

// Graph is a reusable orchestration protocol.
type Graph struct {
	Start string `json:"start"` // entry node id
	Nodes []Node `json:"nodes"`

	// Accumulate turns on conversation-carrying execution: sequential agent nodes
	// append their prompt + reply to State.Thread and are invoked with that growing
	// thread, so the provider's prompt cache reuses the stable prefix across nodes
	// (see State.Thread). A node can opt out with Node.Fresh. NOT omitempty: the UI
	// defaults this ON for flows that never set it, so `false` must persist verbatim
	// to survive a save/reload round-trip (an omitted false would read back as ON).
	Accumulate bool `json:"accumulate"`

	// Presentation hints for the visual canvas builder (cosmetic only — ignored
	// by the engine and Validate). Persisted so a flow keeps its look.
	EdgeStyle string `json:"edgeStyle,omitempty"` // default|smoothstep|step|straight
	Animated  bool   `json:"animated,omitempty"`  // animate edge flow
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
	// Fresh (accumulate-mode only) opts this agent node out of the shared thread:
	// it runs as a stateless single-message call and does not append to State.Thread,
	// so a node that must not see (or grow) the accumulated context stays isolated.
	// Ignored when the graph's Accumulate is off.
	Fresh bool `json:"fresh,omitempty"`
	// OutputSchema (optional) constrains the node's reply to this JSON Schema
	// via structured outputs on providers/models that support it. Pair with a
	// downstream branch node's JSONField for parse-proof routing decisions.
	// Unsupported providers reply free-text — schemas must stay advisory there.
	OutputSchema string `json:"outputSchema,omitempty"`

	// branch — Branches are the routing arms; an empty Contains is the default
	// arm. MatchMode decides how Contains is compared to the last output:
	// "" / "contains" (case-insensitive substring), "equals" (case-insensitive,
	// trimmed exact), or "regex" (Go regexp on the raw output).
	Branches  []Branch `json:"branches,omitempty"`
	MatchMode string   `json:"matchMode,omitempty"`
	// JSONField (optional, branch): when set, the matched value is the named
	// top-level field of the last output parsed as JSON (e.g. "verdict" over
	// {"verdict":"SHIP"}) instead of the raw text — the structured-outputs
	// counterpart of the string modes. Falls back to the raw text when the last
	// output is not valid JSON or lacks the field.
	JSONField string `json:"jsonField,omitempty"`

	// parallel
	Parallel []string `json:"parallel,omitempty"` // agent node ids to run concurrently
	JoinNext string   `json:"joinNext,omitempty"` // node after the join ("" = end)

	// delay
	DelayMs int `json:"delayMs,omitempty"` // milliseconds to wait before Next

	// transform — a template rendered as the node's output (no LLM).
	Template string `json:"template,omitempty"` // {{input}}, {{last}}, {{node.<id>}}

	// loop — repeat the Body sub-chain (a chain that ends at a terminal node,
	// Next="") until an exit condition holds, then continue at LoopNext. One of
	// MaxIters>0 or a non-empty Until is required (Validate enforces it) so a loop
	// can never run forever. Body nodes can read {{iteration}} (0-based). Exit when
	// iteration reaches MaxIters OR Until matches the last output under UntilMode.
	Body      string `json:"body,omitempty"`      // entry node id of the loop body
	LoopNext  string `json:"loopNext,omitempty"`  // node after the loop exits ("" = end)
	MaxIters  int    `json:"maxIters,omitempty"`  // hard iteration cap (0 = rely on Until)
	Until     string `json:"until,omitempty"`     // exit when {{last}} matches this
	UntilMode string `json:"untilMode,omitempty"` // contains|equals|regex ("" = contains)

	// layout (cosmetic only — ignored by the engine and Validate). Persisted so
	// the visual canvas builder can restore node positions across reloads.
	X float64 `json:"x,omitempty"`
	Y float64 `json:"y,omitempty"`
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

// NodeByID is the exported lookup for a node by id (used by callers outside this
// package, e.g. attributing a flow run's output to a specific node's agent).
func (g Graph) NodeByID(id string) (Node, bool) { return g.node(id) }

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
		case NodeDelay, NodeTransform:
			if err := ref(n.Next, "node "+n.ID); err != nil {
				return err
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
		case NodeLoop:
			if n.Body == "" {
				return fmt.Errorf("loop node %q has no body", n.ID)
			}
			if err := ref(n.Body, "body of "+n.ID); err != nil {
				return err
			}
			if err := ref(n.LoopNext, "loopNext of "+n.ID); err != nil {
				return err
			}
			// A loop with neither a positive cap nor an exit condition could run
			// until the global step cap and fail — reject it up front.
			if n.MaxIters <= 0 && n.Until == "" {
				return fmt.Errorf("loop node %q needs maxIters>0 or a non-empty until", n.ID)
			}
		default:
			return fmt.Errorf("node %q has unknown type %q", n.ID, n.Type)
		}
	}
	return nil
}
