package orchestration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// maxSteps bounds total node executions so a cyclic graph (loops are allowed)
// can never run forever.
const maxSteps = 50

// AgentRunner executes one agent node. The runtime implements this; the engine
// stays free of any LLM/provider dependency.
type AgentRunner interface {
	RunAgentNode(ctx context.Context, agentID, prompt string) (string, error)
}

// NodeEvent reports a node's lifecycle to an Observer so a caller can stream
// progress (e.g. over SSE) as the graph executes. Output is populated on "done".
type NodeEvent struct {
	Phase  string `json:"phase"` // "start" | "done"
	NodeID string `json:"nodeId"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Index  int    `json:"index"`            // 1-based execution order
	Output string `json:"output,omitempty"` // on "done"
}

// Observer receives node lifecycle events during a run. It MAY be called
// concurrently (parallel-node children run in goroutines), so implementations
// must be safe for concurrent use.
type Observer func(NodeEvent)

// TraceEntry records one executed node for display and debugging.
type TraceEntry struct {
	NodeID string `json:"nodeId"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Output string `json:"output"`
	At     int64  `json:"at"`
}

// State is the restart-safe snapshot persisted after every node. A run can be
// resumed from any saved State.
type State struct {
	Current string            `json:"current"` // next node to run ("" = finished)
	Last    string            `json:"last"`    // most recent output (branch routing + {{last}})
	Outputs map[string]string `json:"outputs"`
	Steps   int               `json:"steps"`
	Trace   []TraceEntry      `json:"trace"`
}

// NewState builds a fresh state starting at the graph entry node.
func NewState(g Graph) State {
	return State{Current: g.Start, Outputs: map[string]string{}}
}

// SaveFunc persists the state mid-run (called after each node).
type SaveFunc func(State) error

// Engine drives a graph to completion using an AgentRunner.
type Engine struct {
	runner   AgentRunner
	observer Observer // optional; nil = no progress events
}

// NewEngine constructs an engine.
func NewEngine(runner AgentRunner) *Engine { return &Engine{runner: runner} }

// SetObserver registers a progress observer (nil clears it). The observer is
// notified at each node's start and completion; see Observer for concurrency.
func (e *Engine) SetObserver(o Observer) { e.observer = o }

// notify reports a node event when an observer is set (title defaults to nodeID).
func (e *Engine) notify(phase string, node Node, index int, output string) {
	if e.observer == nil {
		return
	}
	title := node.Title
	if title == "" {
		title = node.ID
	}
	e.observer(NodeEvent{Phase: phase, NodeID: node.ID, Type: node.Type, Title: title, Index: index, Output: output})
}

// Run advances the graph from st.Current until it finishes (Current == ""),
// hits the step cap, or an agent errors. It persists after each node via save.
// The returned State is terminal; the final output is State.Last.
func (e *Engine) Run(ctx context.Context, g Graph, input string, st State, save SaveFunc) (State, error) {
	if st.Outputs == nil {
		st.Outputs = map[string]string{}
	}
	for st.Current != "" {
		if st.Steps >= maxSteps {
			return st, fmt.Errorf("step cap (%d) reached — possible runaway loop", maxSteps)
		}
		node, ok := g.node(st.Current)
		if !ok {
			return st, fmt.Errorf("node %q not found", st.Current)
		}
		st.Steps++

		switch node.Type {
		case NodeAgent:
			e.notify("start", node, st.Steps, "")
			prompt := render(node.Prompt, input, st)
			out, err := e.runner.RunAgentNode(ctx, node.AgentID, prompt)
			if err != nil {
				return st, fmt.Errorf("node %q (agent): %w", node.ID, err)
			}
			st.Outputs[node.ID] = out
			st.Last = out
			st.appendTrace(node, out)
			e.notify("done", node, st.Steps, out)
			st.Current = node.Next

		case NodeBranch:
			next, label := evalBranch(node, st.Last)
			st.appendTrace(node, "→ "+label)
			e.notify("done", node, st.Steps, "→ "+label)
			st.Current = next

		case NodeParallel:
			combined, err := e.runParallel(ctx, g, node, input, st)
			if err != nil {
				return st, err
			}
			st.Last = combined
			st.appendTrace(node, combined)
			st.Current = node.JoinNext

		default:
			return st, fmt.Errorf("node %q has unknown type %q", node.ID, node.Type)
		}

		if save != nil {
			if err := save(st); err != nil {
				return st, fmt.Errorf("persist state: %w", err)
			}
		}
	}
	return st, nil
}

// runParallel executes a parallel node's children concurrently and joins their
// outputs. Each child receives the same incoming value (st.Last) as {{last}}.
func (e *Engine) runParallel(ctx context.Context, g Graph, node Node, input string, st State) (string, error) {
	type res struct {
		id, title, out string
		err            error
	}
	results := make([]res, len(node.Parallel))
	var wg sync.WaitGroup

	for i, childID := range node.Parallel {
		child, ok := g.node(childID)
		if !ok {
			return "", fmt.Errorf("parallel child %q not found", childID)
		}
		e.notify("start", child, st.Steps, "")
		wg.Add(1)
		go func(i int, child Node) {
			defer wg.Done()
			prompt := render(child.Prompt, input, st)
			out, err := e.runner.RunAgentNode(ctx, child.AgentID, prompt)
			title := child.Title
			if title == "" {
				title = child.ID
			}
			if err == nil {
				e.notify("done", child, st.Steps, out)
			}
			results[i] = res{id: child.ID, title: title, out: out, err: err}
		}(i, child)
	}
	wg.Wait()

	var b strings.Builder
	for i := range results {
		r := results[i]
		if r.err != nil {
			return "", fmt.Errorf("parallel child %q: %w", r.id, r.err)
		}
		st.Outputs[r.id] = r.out
		fmt.Fprintf(&b, "[%s]\n%s\n\n", r.title, r.out)
	}
	return strings.TrimSpace(b.String()), nil
}

// appendTrace records a node execution.
func (st *State) appendTrace(node Node, output string) {
	title := node.Title
	if title == "" {
		title = node.ID
	}
	st.Trace = append(st.Trace, TraceEntry{
		NodeID: node.ID,
		Type:   node.Type,
		Title:  title,
		Output: output,
		At:     time.Now().Unix(),
	})
}

// evalBranch picks the first matching branch (case-insensitive substring of the
// incoming value). An empty Contains is the default arm. Returns (nextID, label).
func evalBranch(node Node, value string) (string, string) {
	lower := strings.ToLower(value)
	for _, b := range node.Branches {
		if b.Contains == "" {
			return b.Next, "default"
		}
		if strings.Contains(lower, strings.ToLower(b.Contains)) {
			return b.Next, b.Contains
		}
	}
	return "", "no match"
}

// render substitutes template placeholders in a prompt:
//
//	{{input}}        the flow's input
//	{{last}}         the most recent node output
//	{{node.<id>}}    a specific node's stored output
func render(tmpl, input string, st State) string {
	out := strings.ReplaceAll(tmpl, "{{input}}", input)
	out = strings.ReplaceAll(out, "{{last}}", st.Last)
	for id, v := range st.Outputs {
		out = strings.ReplaceAll(out, "{{node."+id+"}}", v)
	}
	return out
}
