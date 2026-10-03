// Package flow is the evolving per-agent flow model: a small directed graph
// (cycles allowed) that every turn of an agent runs through — input node, a few
// llm / route / transform stages, output node. The package is a LEAF (no
// internal imports) so db, agent, tools and api can all depend on it.
//
// Design notes:
//   - Edges are explicit (not "next" pointers) so loops and feedback arms are
//     ordinary edges; a route node picks one of its outgoing edges by label.
//   - Termination is enforced in code, not by the prompt: a global step cap and a
//     per-route visit cap that forces the route's default arm.
//   - Graphs are versioned by the store; this package only knows how to parse,
//     validate, patch, diff and run one graph.
package flow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Node types.
const (
	NodeInput     = "input"     // the turn's input; exactly one per graph
	NodeLLM       = "llm"       // one model call (the owner agent by default)
	NodeRoute     = "route"     // pick an outgoing edge by matching the last output
	NodeTransform = "transform" // render a template, no model call
	NodeTrigger   = "trigger"   // fire an automation with a rendered payload; passes the last output through
	NodeOutput    = "output"    // the turn's reply; exactly one per graph
)

// Context modes of an llm node.
const (
	ContextThread = "thread" // the session history + the rendered prompt as the new user turn
	ContextFresh  = "fresh"  // only the agent's system prompt + the rendered prompt
)

// Tool modes of an llm node.
const (
	ToolsInherit = "inherit" // the agent's normal tool set
	ToolsNone    = "none"    // a plain completion
)

// Route match modes.
const (
	ModeContains = "contains" // case-insensitive substring (default)
	ModeEquals   = "equals"   // case-insensitive, trimmed equality
	ModeRegex    = "regex"    // Go regexp on the raw value
	ModeJSON     = "json"     // equality on a top-level JSON field (JSONField)
	ModeJudge    = "judge"    // a decision model picks the arm whose label describes the value
	ModeCriteria = "criteria" // a decision model checks every criterion; all hold → the "pass" arm, else "fail"
)

// Arm labels a criteria route uses (case-insensitive).
const (
	ArmPass = "pass"
	ArmFail = "fail"
)

// Caps. MaxSteps bounds one run; HardMaxSteps bounds what a graph may ask for;
// HardMaxNodes bounds what any editor (user, agent, observer) may build.
const (
	SchemaVersion    = 2
	DefaultMaxSteps  = 24
	HardMaxSteps     = 100
	HardMaxNodes     = 48
	DefaultMaxVisits = 3
)

// Graph is one version of an agent's flow.
type Graph struct {
	Version  int    `json:"version"`
	Nodes    []Node `json:"nodes"`
	Edges    []Edge `json:"edges"`
	MaxSteps int    `json:"maxSteps,omitempty"`
}

// Node is one stage. Fields are interpreted per Type; unused fields stay empty.
type Node struct {
	ID    string  `json:"id"`
	Type  string  `json:"type"`
	Title string  `json:"title,omitempty"`
	Note  string  `json:"note,omitempty"` // free-form rationale (why this stage exists)
	X     float64 `json:"x,omitempty"`
	Y     float64 `json:"y,omitempty"`

	// llm
	AgentID      string `json:"agentId,omitempty"` // "" = the flow's owner agent
	Prompt       string `json:"prompt,omitempty"`  // template; "" = {{input}}
	Context      string `json:"context,omitempty"` // thread (default) | fresh
	Tools        string `json:"tools,omitempty"`   // inherit (default) | none
	OutputSchema string `json:"outputSchema,omitempty"`
	Model        string `json:"model,omitempty"` // optional model override

	// route
	Mode      string   `json:"mode,omitempty"`      // contains (default) | equals | regex | json | judge | criteria
	JSONField string   `json:"jsonField,omitempty"` // json mode: top-level field of the value
	Question  string   `json:"question,omitempty"`  // judge mode: what to look at
	Criteria  []string `json:"criteria,omitempty"`  // criteria mode: yes/no statements the value must satisfy
	MaxVisits int      `json:"maxVisits,omitempty"` // loop cap; 0 = DefaultMaxVisits

	// trigger
	AutomationID string `json:"automationId,omitempty"` // the automation to fire

	// transform / output / trigger
	Template string `json:"template,omitempty"` // "" on output and trigger = {{last}}
}

// Edge connects two nodes. When is the route arm label ("" = the default arm);
// it is ignored on edges that do not leave a route node.
type Edge struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	When string `json:"when,omitempty"`
}

// Parse decodes a graph from JSON and normalizes it.
func Parse(data string) (Graph, error) {
	var g Graph
	if strings.TrimSpace(data) == "" {
		return g, fmt.Errorf("empty graph")
	}
	if err := json.Unmarshal([]byte(data), &g); err != nil {
		return g, fmt.Errorf("invalid graph JSON: %w", err)
	}
	return g.Normalized(), nil
}

// Encode serializes a graph as compact JSON.
func Encode(g Graph) string {
	b, _ := json.Marshal(g)
	return string(b)
}

// Normalized fills defaults (schema version, trimmed ids, default modes) without
// changing meaning, so stored graphs compare stably.
func (g Graph) Normalized() Graph {
	g.Version = SchemaVersion
	for i := range g.Nodes {
		n := &g.Nodes[i]
		n.ID = strings.TrimSpace(n.ID)
		n.Type = strings.ToLower(strings.TrimSpace(n.Type))
		n.Title = strings.TrimSpace(n.Title)
		switch n.Type {
		case NodeLLM:
			if n.Context == "" {
				n.Context = ContextThread
			}
			if n.Tools == "" {
				n.Tools = ToolsInherit
			}
		case NodeRoute:
			if n.Mode == "" {
				n.Mode = ModeContains
			}
			var crit []string
			for _, c := range n.Criteria {
				if c = strings.TrimSpace(c); c != "" {
					crit = append(crit, c)
				}
			}
			n.Criteria = crit
		case NodeTrigger:
			n.AutomationID = strings.TrimSpace(n.AutomationID)
		}
	}
	for i := range g.Edges {
		e := &g.Edges[i]
		e.ID = strings.TrimSpace(e.ID)
		e.From = strings.TrimSpace(e.From)
		e.To = strings.TrimSpace(e.To)
		e.When = strings.TrimSpace(e.When)
		if e.ID == "" {
			e.ID = "e_" + e.From + "_" + e.To
		}
	}
	return g
}

// DefaultGraph is the flow every agent starts with: input → respond → output.
// Running it is byte-for-byte the plain chat turn.
func DefaultGraph() Graph {
	return Graph{
		Version: SchemaVersion,
		Nodes: []Node{
			{ID: "input", Type: NodeInput, Title: "Girdi", X: 0, Y: 0},
			{ID: "respond", Type: NodeLLM, Title: "Yanıt", Prompt: "{{input}}", Context: ContextThread, Tools: ToolsInherit, X: 0, Y: 150},
			{ID: "output", Type: NodeOutput, Title: "Çıktı", Template: "{{last}}", X: 0, Y: 300},
		},
		Edges: []Edge{
			{ID: "e_input_respond", From: "input", To: "respond"},
			{ID: "e_respond_output", From: "respond", To: "output"},
		},
	}
}

// IsTrivial reports whether the graph is behaviourally identical to a plain
// chat turn: input → one thread-mode llm node on the owner agent with the raw
// input as prompt → output passing it through. The runtime skips the per-node
// chrome (node step cards) for such flows.
func (g Graph) IsTrivial() bool {
	if len(g.Nodes) != 3 || len(g.Edges) != 2 {
		return false
	}
	in, ok := g.single(NodeInput)
	if !ok {
		return false
	}
	out, ok := g.single(NodeOutput)
	if !ok {
		return false
	}
	llm, ok := g.single(NodeLLM)
	if !ok {
		return false
	}
	if !g.hasEdge(in.ID, llm.ID) || !g.hasEdge(llm.ID, out.ID) {
		return false
	}
	p := strings.TrimSpace(llm.Prompt)
	if p != "" && p != "{{input}}" {
		return false
	}
	if llm.AgentID != "" || llm.Model != "" || strings.TrimSpace(llm.OutputSchema) != "" {
		return false
	}
	if llm.Context != "" && llm.Context != ContextThread {
		return false
	}
	if llm.Tools != "" && llm.Tools != ToolsInherit {
		return false
	}
	t := strings.TrimSpace(out.Template)
	return t == "" || t == "{{last}}"
}

func (g Graph) single(typ string) (Node, bool) {
	var found Node
	n := 0
	for _, x := range g.Nodes {
		if x.Type == typ {
			found = x
			n++
		}
	}
	return found, n == 1
}

func (g Graph) hasEdge(from, to string) bool {
	for _, e := range g.Edges {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}

// NodeByID returns the node with the given id.
func (g Graph) NodeByID(id string) (Node, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

// EdgeByID returns the edge with the given id.
func (g Graph) EdgeByID(id string) (Edge, bool) {
	for _, e := range g.Edges {
		if e.ID == id {
			return e, true
		}
	}
	return Edge{}, false
}

// Outgoing lists the edges leaving a node, in graph order.
func (g Graph) Outgoing(id string) []Edge {
	var out []Edge
	for _, e := range g.Edges {
		if e.From == id {
			out = append(out, e)
		}
	}
	return out
}

// Incoming lists the edges entering a node, in graph order.
func (g Graph) Incoming(id string) []Edge {
	var out []Edge
	for _, e := range g.Edges {
		if e.To == id {
			out = append(out, e)
		}
	}
	return out
}

// Input returns the graph's input node (Validate guarantees exactly one).
func (g Graph) Input() (Node, bool) { return g.single(NodeInput) }

// Output returns the graph's output node (Validate guarantees exactly one).
func (g Graph) Output() (Node, bool) { return g.single(NodeOutput) }

// EffectiveMaxSteps resolves the step cap with defaults and the hard ceiling.
func (g Graph) EffectiveMaxSteps() int {
	if g.MaxSteps <= 0 {
		return DefaultMaxSteps
	}
	if g.MaxSteps > HardMaxSteps {
		return HardMaxSteps
	}
	return g.MaxSteps
}

// MaxVisitsOf resolves a route node's loop cap.
func MaxVisitsOf(n Node) int {
	if n.MaxVisits <= 0 {
		return DefaultMaxVisits
	}
	return n.MaxVisits
}

var validModes = map[string]bool{ModeContains: true, ModeEquals: true, ModeRegex: true, ModeJSON: true, ModeJudge: true, ModeCriteria: true}

// MaxCriteria bounds a criteria route (one decision question per criterion).
const MaxCriteria = 12

// Validate checks the structural invariants every stored graph must satisfy:
// exactly one input and one output, unique ids, resolvable edges, one outgoing
// edge on linear nodes, labelled arms on route nodes, every node reachable from
// the input and able to reach the output, bounded loops (every cycle passes
// through a route node that has a default arm), valid templates, and the hard
// size caps. A graph that passes here can always terminate.
func (g Graph) Validate() error {
	g = g.Normalized()
	if len(g.Nodes) == 0 {
		return fmt.Errorf("graph has no nodes")
	}
	if len(g.Nodes) > HardMaxNodes {
		return fmt.Errorf("graph has %d nodes, the cap is %d", len(g.Nodes), HardMaxNodes)
	}
	if g.MaxSteps > HardMaxSteps {
		return fmt.Errorf("maxSteps %d exceeds the cap %d", g.MaxSteps, HardMaxSteps)
	}
	ids := map[string]bool{}
	inputs, outputs := 0, 0
	for _, n := range g.Nodes {
		if n.ID == "" {
			return fmt.Errorf("node with empty id")
		}
		if ids[n.ID] {
			return fmt.Errorf("duplicate node id %q", n.ID)
		}
		ids[n.ID] = true
		switch n.Type {
		case NodeInput:
			inputs++
		case NodeOutput:
			outputs++
		case NodeLLM:
			if n.Context != ContextThread && n.Context != ContextFresh {
				return fmt.Errorf("node %q: context must be %q or %q", n.ID, ContextThread, ContextFresh)
			}
			if n.Tools != ToolsInherit && n.Tools != ToolsNone {
				return fmt.Errorf("node %q: tools must be %q or %q", n.ID, ToolsInherit, ToolsNone)
			}
			if s := strings.TrimSpace(n.OutputSchema); s != "" && !json.Valid([]byte(s)) {
				return fmt.Errorf("node %q: outputSchema must be valid JSON", n.ID)
			}
		case NodeRoute:
			if !validModes[n.Mode] {
				return fmt.Errorf("node %q: unknown route mode %q", n.ID, n.Mode)
			}
			if n.Mode == ModeJSON && strings.TrimSpace(n.JSONField) == "" {
				return fmt.Errorf("node %q: json mode needs jsonField", n.ID)
			}
			if n.Mode == ModeCriteria {
				if len(n.Criteria) == 0 {
					return fmt.Errorf("node %q: criteria mode needs at least one criterion", n.ID)
				}
				if len(n.Criteria) > MaxCriteria {
					return fmt.Errorf("node %q: at most %d criteria", n.ID, MaxCriteria)
				}
			}
		case NodeTransform:
			if strings.TrimSpace(n.Template) == "" {
				return fmt.Errorf("node %q: transform needs a template", n.ID)
			}
		case NodeTrigger:
			if n.AutomationID == "" {
				return fmt.Errorf("node %q: trigger needs automationId", n.ID)
			}
		default:
			return fmt.Errorf("node %q has unknown type %q", n.ID, n.Type)
		}
	}
	if inputs != 1 {
		return fmt.Errorf("graph must have exactly one input node, found %d", inputs)
	}
	if outputs != 1 {
		return fmt.Errorf("graph must have exactly one output node, found %d", outputs)
	}
	edgeIDs := map[string]bool{}
	for _, e := range g.Edges {
		if edgeIDs[e.ID] {
			return fmt.Errorf("duplicate edge id %q", e.ID)
		}
		edgeIDs[e.ID] = true
		if !ids[e.From] {
			return fmt.Errorf("edge %q leaves unknown node %q", e.ID, e.From)
		}
		if !ids[e.To] {
			return fmt.Errorf("edge %q enters unknown node %q", e.ID, e.To)
		}
		if e.From == e.To {
			return fmt.Errorf("edge %q is a self-loop on %q", e.ID, e.From)
		}
	}
	for _, n := range g.Nodes {
		out := g.Outgoing(n.ID)
		switch n.Type {
		case NodeInput:
			if len(g.Incoming(n.ID)) != 0 {
				return fmt.Errorf("input node %q must have no incoming edge", n.ID)
			}
			if len(out) != 1 {
				return fmt.Errorf("input node %q must have exactly one outgoing edge", n.ID)
			}
		case NodeOutput:
			if len(out) != 0 {
				return fmt.Errorf("output node %q must have no outgoing edge", n.ID)
			}
		case NodeLLM, NodeTransform, NodeTrigger:
			if len(out) != 1 {
				return fmt.Errorf("node %q must have exactly one outgoing edge, has %d", n.ID, len(out))
			}
		case NodeRoute:
			if len(out) == 0 {
				return fmt.Errorf("route node %q has no outgoing edge", n.ID)
			}
			defaults := 0
			seen := map[string]bool{}
			for _, e := range out {
				if e.When == "" {
					defaults++
					continue
				}
				key := strings.ToLower(e.When)
				if seen[key] {
					return fmt.Errorf("route node %q has two arms labelled %q", n.ID, e.When)
				}
				seen[key] = true
				if n.Mode == ModeRegex {
					if _, err := regexp.Compile(e.When); err != nil {
						return fmt.Errorf("route node %q arm %q: invalid regexp: %v", n.ID, e.When, err)
					}
				}
			}
			if defaults > 1 {
				return fmt.Errorf("route node %q has more than one default arm", n.ID)
			}
			if n.Mode == ModeJudge && len(out)-defaults == 0 {
				return fmt.Errorf("route node %q uses judge mode but no arm has a label", n.ID)
			}
			if n.Mode == ModeCriteria && !seen[ArmPass] && !seen[ArmFail] {
				return fmt.Errorf("route node %q uses criteria mode but has no arm labelled %q or %q", n.ID, ArmPass, ArmFail)
			}
		}
	}
	in, _ := g.Input()
	outNode, _ := g.Output()
	reach := g.reachableFrom(in.ID)
	for _, n := range g.Nodes {
		if !reach[n.ID] {
			return fmt.Errorf("node %q is not reachable from the input", n.ID)
		}
	}
	back := g.reachableTo(outNode.ID)
	for _, n := range g.Nodes {
		if !back[n.ID] {
			return fmt.Errorf("node %q can never reach the output", n.ID)
		}
	}
	// Every cycle must pass through a route node that has a default arm:
	// without one the visit cap has nowhere to exit to.
	if cyc := g.cycleWithoutExit(); cyc != "" {
		return fmt.Errorf("loop through %q has no route node with a default arm to exit through", cyc)
	}
	// Templates may only reference nodes that exist.
	for _, n := range g.Nodes {
		for _, tpl := range []string{n.Prompt, n.Template} {
			for _, ref := range nodeRefs(tpl) {
				if !ids[ref] {
					return fmt.Errorf("node %q references unknown node {{node.%s}}", n.ID, ref)
				}
			}
		}
	}
	return nil
}

func (g Graph) reachableFrom(start string) map[string]bool {
	seen := map[string]bool{}
	stack := []string{start}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		for _, e := range g.Outgoing(cur) {
			stack = append(stack, e.To)
		}
	}
	return seen
}

func (g Graph) reachableTo(target string) map[string]bool {
	seen := map[string]bool{}
	stack := []string{target}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		for _, e := range g.Incoming(cur) {
			stack = append(stack, e.From)
		}
	}
	return seen
}

// cycleWithoutExit returns a node id on a cycle that contains no route node with
// a default arm, or "" when every cycle has such an exit. Implemented by
// deleting the exit-capable route nodes and looking for any remaining cycle.
func (g Graph) cycleWithoutExit() string {
	exit := map[string]bool{}
	for _, n := range g.Nodes {
		if n.Type != NodeRoute {
			continue
		}
		for _, e := range g.Outgoing(n.ID) {
			if e.When == "" {
				exit[n.ID] = true
			}
		}
	}
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[string]int{}
	var visit func(id string) string
	visit = func(id string) string {
		color[id] = grey
		for _, e := range g.Outgoing(id) {
			if exit[e.To] {
				continue
			}
			switch color[e.To] {
			case grey:
				return e.To
			case white:
				if c := visit(e.To); c != "" {
					return c
				}
			}
		}
		color[id] = black
		return ""
	}
	// Deterministic order so the reported node is stable.
	ids := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		ids = append(ids, n.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if exit[id] || color[id] != white {
			continue
		}
		if c := visit(id); c != "" {
			return c
		}
	}
	return ""
}

// Summary renders the graph as one line for agents and lists:
// "input → plan(llm) → respond(llm) → check(route: APPROVE→output, *→respond) → output".
func (g Graph) Summary() string {
	in, ok := g.Input()
	if !ok {
		return fmt.Sprintf("%d nodes, %d edges", len(g.Nodes), len(g.Edges))
	}
	var parts []string
	seen := map[string]bool{}
	cur := in.ID
	for cur != "" && !seen[cur] {
		seen[cur] = true
		n, _ := g.NodeByID(cur)
		label := n.ID
		switch n.Type {
		case NodeInput, NodeOutput:
			label = n.Type
		case NodeRoute:
			var arms []string
			for _, e := range g.Outgoing(n.ID) {
				w := e.When
				if w == "" {
					w = "*"
				}
				arms = append(arms, w+"→"+e.To)
			}
			label = fmt.Sprintf("%s(route: %s)", n.ID, strings.Join(arms, ", "))
		default:
			label = fmt.Sprintf("%s(%s)", n.ID, n.Type)
		}
		parts = append(parts, label)
		next := ""
		for _, e := range g.Outgoing(cur) {
			if n.Type != NodeRoute || e.When == "" || next == "" {
				if !seen[e.To] {
					next = e.To
					if n.Type != NodeRoute {
						break
					}
				}
			}
		}
		cur = next
	}
	for _, n := range g.Nodes {
		if !seen[n.ID] {
			parts = append(parts, fmt.Sprintf("%s(%s)", n.ID, n.Type))
		}
	}
	return strings.Join(parts, " → ")
}

// DiffSummary names what changed between two versions of a graph.
type DiffSummary struct {
	AddedNodes   []string `json:"addedNodes,omitempty"`
	RemovedNodes []string `json:"removedNodes,omitempty"`
	ChangedNodes []string `json:"changedNodes,omitempty"`
	AddedEdges   []string `json:"addedEdges,omitempty"`
	RemovedEdges []string `json:"removedEdges,omitempty"`
}

// Empty reports whether nothing changed.
func (d DiffSummary) Empty() bool {
	return len(d.AddedNodes)+len(d.RemovedNodes)+len(d.ChangedNodes)+len(d.AddedEdges)+len(d.RemovedEdges) == 0
}

// String renders the diff compactly ("+critic, ~respond, +e_critic_check").
func (d DiffSummary) String() string {
	var parts []string
	for _, n := range d.AddedNodes {
		parts = append(parts, "+"+n)
	}
	for _, n := range d.RemovedNodes {
		parts = append(parts, "-"+n)
	}
	for _, n := range d.ChangedNodes {
		parts = append(parts, "~"+n)
	}
	for _, e := range d.AddedEdges {
		parts = append(parts, "+edge "+e)
	}
	for _, e := range d.RemovedEdges {
		parts = append(parts, "-edge "+e)
	}
	if len(parts) == 0 {
		return "no change"
	}
	return strings.Join(parts, ", ")
}

// Diff compares two graphs node-by-node and edge-by-edge. Positions (X/Y) are
// cosmetic and ignored.
func Diff(a, b Graph) DiffSummary {
	var d DiffSummary
	an := map[string]Node{}
	for _, n := range a.Nodes {
		an[n.ID] = n
	}
	bn := map[string]Node{}
	for _, n := range b.Nodes {
		bn[n.ID] = n
	}
	for _, n := range b.Nodes {
		old, ok := an[n.ID]
		if !ok {
			d.AddedNodes = append(d.AddedNodes, n.ID)
			continue
		}
		if !sameNode(old, n) {
			d.ChangedNodes = append(d.ChangedNodes, n.ID)
		}
	}
	for _, n := range a.Nodes {
		if _, ok := bn[n.ID]; !ok {
			d.RemovedNodes = append(d.RemovedNodes, n.ID)
		}
	}
	ae := map[string]Edge{}
	for _, e := range a.Edges {
		ae[edgeKey(e)] = e
	}
	be := map[string]Edge{}
	for _, e := range b.Edges {
		be[edgeKey(e)] = e
	}
	for k, e := range be {
		if _, ok := ae[k]; !ok {
			d.AddedEdges = append(d.AddedEdges, e.From+"→"+e.To)
		}
	}
	for k, e := range ae {
		if _, ok := be[k]; !ok {
			d.RemovedEdges = append(d.RemovedEdges, e.From+"→"+e.To)
		}
	}
	sort.Strings(d.AddedEdges)
	sort.Strings(d.RemovedEdges)
	return d
}

func edgeKey(e Edge) string { return e.From + "\x00" + e.To + "\x00" + e.When }

func sameNode(a, b Node) bool {
	a.X, a.Y, b.X, b.Y = 0, 0, 0, 0
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

var nodeRefRe = regexp.MustCompile(`\{\{\s*node\.([A-Za-z0-9_\-]+)\s*\}\}`)

// nodeRefs lists the node ids a template references through {{node.<id>}}.
func nodeRefs(tpl string) []string {
	var out []string
	for _, m := range nodeRefRe.FindAllStringSubmatch(tpl, -1) {
		out = append(out, m[1])
	}
	return out
}
