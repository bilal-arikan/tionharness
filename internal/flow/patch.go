package flow

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Op is one edit of a graph. Agents, the observer and the UI all speak this
// vocabulary, so every mutation path validates the same way.
//
//	add_node       {node}                          — append a node (id auto-assigned when empty)
//	update_node    {id, fields}                    — merge fields into a node (type/id immutable)
//	remove_node    {id}                            — drop a node; a linear node is bridged (in→out)
//	add_edge       {edge}                          — connect two nodes (when = route arm label)
//	update_edge    {id, edge}                      — change an edge's endpoints/label
//	remove_edge    {id}
//	insert_between {from, to, node}                — put a new node on the from→to edge
//	set_max_steps  {maxSteps}
type Op struct {
	Op       string          `json:"op"`
	ID       string          `json:"id,omitempty"`
	Node     *Node           `json:"node,omitempty"`
	Edge     *Edge           `json:"edge,omitempty"`
	Fields   json.RawMessage `json:"fields,omitempty"`
	From     string          `json:"from,omitempty"`
	To       string          `json:"to,omitempty"`
	MaxSteps int             `json:"maxSteps,omitempty"`
}

// Apply applies ops to a copy of g in order and validates the result. The
// returned graph is normalized; g is never mutated. An invalid intermediate
// state is fine as long as the final graph validates.
func Apply(g Graph, ops []Op) (Graph, error) {
	out := clone(g)
	for i, op := range ops {
		var err error
		switch strings.ToLower(strings.TrimSpace(op.Op)) {
		case "add_node":
			err = out.addNode(op)
		case "update_node":
			err = out.updateNode(op)
		case "remove_node":
			err = out.removeNode(op)
		case "add_edge":
			err = out.addEdge(op)
		case "update_edge":
			err = out.updateEdge(op)
		case "remove_edge":
			err = out.removeEdge(op)
		case "insert_between":
			err = out.insertBetween(op)
		case "set_max_steps":
			out.MaxSteps = op.MaxSteps
		default:
			err = fmt.Errorf("unknown op %q", op.Op)
		}
		if err != nil {
			return Graph{}, fmt.Errorf("op %d (%s): %w", i+1, op.Op, err)
		}
	}
	out = out.Normalized()
	if err := out.Validate(); err != nil {
		return Graph{}, err
	}
	return out, nil
}

func clone(g Graph) Graph {
	c := g
	c.Nodes = append([]Node(nil), g.Nodes...)
	c.Edges = append([]Edge(nil), g.Edges...)
	return c
}

func (g *Graph) addNode(op Op) error {
	if op.Node == nil {
		return fmt.Errorf("node is required")
	}
	n := *op.Node
	n.ID = strings.TrimSpace(n.ID)
	if n.ID == "" {
		n.ID = g.freshID(strings.ToLower(strings.TrimSpace(n.Type)))
	}
	if _, exists := g.NodeByID(n.ID); exists {
		return fmt.Errorf("node %q already exists", n.ID)
	}
	g.Nodes = append(g.Nodes, n)
	return nil
}

func (g *Graph) updateNode(op Op) error {
	idx := g.nodeIndex(op.ID)
	if idx < 0 {
		return fmt.Errorf("unknown node %q", op.ID)
	}
	if len(op.Fields) == 0 {
		return fmt.Errorf("fields are required")
	}
	cur := g.Nodes[idx]
	raw, _ := json.Marshal(cur)
	var merged map[string]json.RawMessage
	_ = json.Unmarshal(raw, &merged)
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(op.Fields, &patch); err != nil {
		return fmt.Errorf("fields must be a JSON object: %w", err)
	}
	for k, v := range patch {
		if k == "id" || k == "type" {
			return fmt.Errorf("field %q is immutable (remove and re-add the node instead)", k)
		}
		merged[k] = v
	}
	out, _ := json.Marshal(merged)
	var next Node
	if err := json.Unmarshal(out, &next); err != nil {
		return fmt.Errorf("fields do not fit a node: %w", err)
	}
	g.Nodes[idx] = next
	return nil
}

func (g *Graph) removeNode(op Op) error {
	idx := g.nodeIndex(op.ID)
	if idx < 0 {
		return fmt.Errorf("unknown node %q", op.ID)
	}
	n := g.Nodes[idx]
	if n.Type == NodeInput || n.Type == NodeOutput {
		return fmt.Errorf("the %s node cannot be removed", n.Type)
	}
	in := g.Incoming(n.ID)
	out := g.Outgoing(n.ID)
	g.Nodes = append(g.Nodes[:idx], g.Nodes[idx+1:]...)
	kept := g.Edges[:0]
	for _, e := range g.Edges {
		if e.From != n.ID && e.To != n.ID {
			kept = append(kept, e)
		}
	}
	g.Edges = kept
	// Bridge: a node with one successor hands every predecessor to it, keeping
	// the arm labels of the incoming edges.
	if len(out) == 1 {
		for _, e := range in {
			if e.From == out[0].To {
				continue // would be a self-loop
			}
			g.Edges = append(g.Edges, Edge{ID: "e_" + e.From + "_" + out[0].To, From: e.From, To: out[0].To, When: e.When})
		}
	}
	return nil
}

func (g *Graph) addEdge(op Op) error {
	if op.Edge == nil {
		return fmt.Errorf("edge is required")
	}
	e := *op.Edge
	e.From, e.To, e.When = strings.TrimSpace(e.From), strings.TrimSpace(e.To), strings.TrimSpace(e.When)
	if _, ok := g.NodeByID(e.From); !ok {
		return fmt.Errorf("unknown node %q", e.From)
	}
	if _, ok := g.NodeByID(e.To); !ok {
		return fmt.Errorf("unknown node %q", e.To)
	}
	if e.ID == "" {
		e.ID = g.freshEdgeID(e.From, e.To)
	}
	if _, exists := g.EdgeByID(e.ID); exists {
		return fmt.Errorf("edge %q already exists", e.ID)
	}
	g.Edges = append(g.Edges, e)
	return nil
}

func (g *Graph) updateEdge(op Op) error {
	if op.Edge == nil {
		return fmt.Errorf("edge is required")
	}
	for i := range g.Edges {
		if g.Edges[i].ID != op.ID {
			continue
		}
		e := g.Edges[i]
		if op.Edge.From != "" {
			e.From = strings.TrimSpace(op.Edge.From)
		}
		if op.Edge.To != "" {
			e.To = strings.TrimSpace(op.Edge.To)
		}
		e.When = strings.TrimSpace(op.Edge.When)
		g.Edges[i] = e
		return nil
	}
	return fmt.Errorf("unknown edge %q", op.ID)
}

func (g *Graph) removeEdge(op Op) error {
	for i := range g.Edges {
		if g.Edges[i].ID == op.ID {
			g.Edges = append(g.Edges[:i], g.Edges[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("unknown edge %q", op.ID)
}

func (g *Graph) insertBetween(op Op) error {
	if op.Node == nil {
		return fmt.Errorf("node is required")
	}
	from, to := strings.TrimSpace(op.From), strings.TrimSpace(op.To)
	var idx = -1
	for i, e := range g.Edges {
		if e.From == from && e.To == to {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("no edge %s→%s to insert on", from, to)
	}
	if err := g.addNode(op); err != nil {
		return err
	}
	n := g.Nodes[len(g.Nodes)-1]
	old := g.Edges[idx]
	g.Edges[idx] = Edge{ID: old.ID, From: from, To: n.ID, When: old.When}
	g.Edges = append(g.Edges, Edge{ID: g.freshEdgeID(n.ID, to), From: n.ID, To: to})
	// Place the new node between its neighbours on the canvas.
	if a, ok := g.NodeByID(from); ok {
		if b, ok := g.NodeByID(to); ok {
			i := g.nodeIndex(n.ID)
			g.Nodes[i].X = (a.X + b.X) / 2
			g.Nodes[i].Y = (a.Y + b.Y) / 2
		}
	}
	return nil
}

func (g Graph) nodeIndex(id string) int {
	for i, n := range g.Nodes {
		if n.ID == id {
			return i
		}
	}
	return -1
}

func (g Graph) freshID(base string) string {
	if base == "" {
		base = "node"
	}
	if _, ok := g.NodeByID(base); !ok {
		return base
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s_%d", base, i)
		if _, ok := g.NodeByID(cand); !ok {
			return cand
		}
	}
}

func (g Graph) freshEdgeID(from, to string) string {
	base := "e_" + from + "_" + to
	if _, ok := g.EdgeByID(base); !ok {
		return base
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s_%d", base, i)
		if _, ok := g.EdgeByID(cand); !ok {
			return cand
		}
	}
}

// ParseOps decodes a JSON array of ops.
func ParseOps(raw json.RawMessage) ([]Op, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("ops are required")
	}
	var ops []Op
	if err := json.Unmarshal(raw, &ops); err != nil {
		return nil, fmt.Errorf("ops must be a JSON array: %w", err)
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("ops are empty")
	}
	return ops, nil
}
