// Package orchestration runs structured multi-agent protocols described as a
// node graph. It supports sequential agent steps, conditional branching, and
// parallel fan-out + join, with restart-safe run state persisted by the caller.
package orchestration

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Node types.
const (
	NodeAgent       = "agent"       // run an agent with a (templated) prompt, then go to Next
	NodeBranch      = "branch"      // route by matching the last output (see MatchMode)
	NodeParallel    = "parallel"    // run several agent nodes concurrently, then JoinNext
	NodeDelay       = "delay"       // wait DelayMs, then go to Next (no LLM)
	NodeTransform   = "transform"   // emit a rendered template as output, then Next (no LLM)
	NodeLoop        = "loop"        // repeat a body sub-chain until MaxIters/Until, then LoopNext
	NodeAwaitInput  = "await-input" // durably suspend until external input arrives, then Next
	NodeSubflow     = "subflow"     // run another flow to completion, capture its output, then Next
	NodeStart       = "start"       // required entry marker; passes straight through to Next
	NodeEnd         = "end"         // optional terminal; may shape (Template) / validate (OutputSchema) the final output
	NodeSpawn       = "spawn"       // launch SpawnFlows as async child runs (non-blocking), then Next
	NodeJoin        = "join"        // barrier: wait for a spawn node's child runs, collect outputs, then Next
	NodeCoordinator = "coordinator" // run an agent as a coordinator that spawns workers, then Next
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

	// start — the required entry node; carries only Next (a pass-through). end — an
	// optional terminal that may render Template as the final output and/or validate
	// the final output against OutputSchema (both fields declared above/for agent).

	// subflow — run another flow (FlowRef) to completion with a rendered input
	// (Template; defaults to {{last}} when empty), capture its final output as this
	// node's output, then continue at Next. Enables flow composition/reuse. The
	// runner guards against runaway recursion (a flow calling itself too deep).
	FlowRef string `json:"flowRef,omitempty"` // id of the child flow to run

	// spawn — launch each flow in SpawnFlows as an INDEPENDENT async child run
	// (non-blocking); the run ids are recorded in State.Spawned[node.ID] and the
	// engine continues at Next immediately. The rendered input (Template, default
	// {{last}}) is passed to every child. A later join node collects the results.
	SpawnFlows []string `json:"spawnFlows,omitempty"`

	// join — barrier that waits for the child runs launched by a spawn node, then
	// continues at Next with their joined outputs as {{last}}. SpawnRef names the
	// spawn node whose children to await ("" = every outstanding spawned run). A
	// spawned child that suspends at await-input fails the join (async children
	// must be non-interactive) so the barrier always terminates.
	SpawnRef string `json:"spawnRef,omitempty"`
	// JoinTimeoutSec bounds how long a join waits (0 = wait forever). JoinPartial
	// makes the join tolerant: a failed/suspended/timed-out child is DROPPED (its
	// output excluded) instead of failing the whole join, so the barrier proceeds
	// with the successful children's outputs. With JoinPartial off (default) any
	// failure/suspension — or the timeout — fails the join.
	JoinTimeoutSec int  `json:"joinTimeoutSec,omitempty"`
	JoinPartial    bool `json:"joinPartial,omitempty"`

	// await-input — TimeoutSec optionally bounds how long the run may stay
	// suspended: a background sweeper fails a waiting run once now-suspendTime
	// exceeds it (0 = wait forever). The engine itself ignores this field.
	//
	// coordinator — TimeoutSec instead bounds how long the node waits for the
	// coordinator to settle (0 = the runner's own default); on expiry the node
	// fails and the runner stops any still-running workers.
	TimeoutSec int `json:"timeoutSec,omitempty"`

	// coordinator — run AgentID as a COORDINATOR session seeded with the rendered
	// Prompt. Unlike a parallel node (whose fan-out width is fixed at design time)
	// the coordinator decides at RUNTIME how many workers to spawn and how to
	// dispatch them; the node blocks until every worker has finished and the
	// coordinator has stopped reacting, then emits the coordinator's final reply as
	// this node's output. MaxTurns caps the coordinator's auto-turn (notify-loop)
	// budget for this node only (0 = the recipe's own cap, else the workspace
	// default). Workflow names a saved coordination recipe (a skill with kind
	// "coordinator-workflow" — the same list the session panel offers) whose body
	// is layered onto the coordinator prompt; "" = free coordination.
	MaxTurns int    `json:"maxTurns,omitempty"`
	Workflow string `json:"workflow,omitempty"`

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

// MigrateAddStart upgrades a graph to the required start-node model: when it has
// no start node, it prepends one whose Next is the old entry (Start) and repoints
// Start to it. Returns the (possibly new) graph and whether it changed. Idempotent.
func MigrateAddStart(g Graph) (Graph, bool) {
	for _, n := range g.Nodes {
		if n.Type == NodeStart {
			return g, false // already has a start node
		}
	}
	id := uniqueNodeID(g, "start")
	var x, y float64
	if old, ok := g.node(g.Start); ok {
		x, y = old.X, old.Y-120
	}
	start := Node{ID: id, Type: NodeStart, Title: "Başlangıç", Next: g.Start, X: x, Y: y}
	g.Nodes = append([]Node{start}, g.Nodes...)
	g.Start = id
	return g, true
}

// uniqueNodeID returns base, or base_1/base_2/… when base is already taken.
func uniqueNodeID(g Graph, base string) string {
	if _, ok := g.node(base); !ok {
		return base
	}
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s_%d", base, i)
		if _, ok := g.node(cand); !ok {
			return cand
		}
	}
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

	// Exactly one start node, and it must be the graph entry. The start node type
	// is the entry marker (no separate "mark as start").
	startCount := 0
	startID := ""
	for _, n := range g.Nodes {
		if n.Type == NodeStart {
			startCount++
			startID = n.ID
		}
	}
	if startCount != 1 {
		return fmt.Errorf("graph must have exactly one start node, found %d", startCount)
	}
	if g.Start != startID {
		return fmt.Errorf("graph entry %q must be the start node %q", g.Start, startID)
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
		case NodeCoordinator:
			if n.AgentID == "" {
				return fmt.Errorf("coordinator node %q has no agentId", n.ID)
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
		case NodeStart, NodeDelay, NodeTransform, NodeAwaitInput:
			if err := ref(n.Next, "node "+n.ID); err != nil {
				return err
			}
		case NodeEnd:
			// Terminal: no outgoing edge. OutputSchema, when set, must be valid JSON.
			if s := strings.TrimSpace(n.OutputSchema); s != "" {
				if !json.Valid([]byte(s)) {
					return fmt.Errorf("end node %q has an invalid OutputSchema (must be JSON)", n.ID)
				}
			}
		case NodeSubflow:
			if n.FlowRef == "" {
				return fmt.Errorf("subflow node %q has no flowRef", n.ID)
			}
			if err := ref(n.Next, "node "+n.ID); err != nil {
				return err
			}
		case NodeSpawn:
			// SpawnFlows are external flow ids (not nodes in this graph) so they are
			// not ref-checked here; existence is verified at launch time.
			if len(n.SpawnFlows) == 0 {
				return fmt.Errorf("spawn node %q has no spawnFlows", n.ID)
			}
			if err := ref(n.Next, "node "+n.ID); err != nil {
				return err
			}
		case NodeJoin:
			if n.SpawnRef != "" {
				sn, ok := g.node(n.SpawnRef)
				if !ok {
					return fmt.Errorf("join node %q references unknown spawn node %q", n.ID, n.SpawnRef)
				}
				if sn.Type != NodeSpawn {
					return fmt.Errorf("join node %q spawnRef %q is not a spawn node", n.ID, n.SpawnRef)
				}
			}
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
