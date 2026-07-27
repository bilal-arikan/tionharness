package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxSteps bounds total node executions so a cyclic graph (loops are allowed)
// can never run forever.
const maxSteps = 50

// maxDelayMs caps a delay node's wait so a misconfiguration can't block a run
// indefinitely (5 minutes).
const maxDelayMs = 5 * 60 * 1000

// AgentRunner executes one agent node. The runtime implements this; the engine
// stays free of any LLM/provider dependency.
type AgentRunner interface {
	RunAgentNode(ctx context.Context, agentID, prompt string) (string, error)
}

// SchemaAgentRunner is an OPTIONAL extension: runners that can constrain an
// agent node's reply to a JSON Schema (structured outputs) implement it. The
// engine uses it only for nodes with a non-empty OutputSchema; plain runners
// keep working unchanged (the schema is then advisory-only).
type SchemaAgentRunner interface {
	RunAgentNodeSchema(ctx context.Context, agentID, prompt, outputSchema string) (string, error)
}

// Msg is one turn of an accumulated conversation thread (see State.Thread).
type Msg struct {
	Role string `json:"role"` // "user" | "assistant"
	Text string `json:"text"`
}

// ChildFlowRunner is an OPTIONAL extension that lets a subflow node run another
// flow to completion and capture its output. Runners that do not implement it
// make a subflow node fail with a clear "not wired" error.
type ChildFlowRunner interface {
	RunChildFlow(ctx context.Context, flowID, input string) (string, error)
}

// SuspendableChildFlowRunner is an OPTIONAL extension of ChildFlowRunner that
// lets a subflow node propagate a child's await-input suspension up to the
// parent flow (instead of failing). RunChildFlowResumable runs the child and, if
// it suspends, returns waiting=true with the child run id; ResumeChildFlow feeds
// the parent's later input to that suspended child and drives it one more step.
// Runners that do not implement it keep the fail-on-suspend behaviour.
type SuspendableChildFlowRunner interface {
	RunChildFlowResumable(ctx context.Context, flowID, input string) (out, childRunID string, waiting bool, err error)
	ResumeChildFlow(ctx context.Context, childRunID, input string) (out string, waiting bool, err error)
}

// AsyncFlowRunner is an OPTIONAL extension powering spawn/join nodes: it launches
// child flows asynchronously (SpawnChildFlows, non-blocking, returns their run
// ids) and later blocks until they finish (JoinChildFlows, returns their outputs
// in order). Runners that do not implement it make a spawn/join node fail clearly.
type AsyncFlowRunner interface {
	SpawnChildFlows(ctx context.Context, flowIDs []string, input string) ([]string, error)
	JoinChildFlows(ctx context.Context, runIDs []string, timeoutSec int, partial bool, onProgress func(done, total int)) (outputs []string, err error)
}

// ThreadAgentRunner is an OPTIONAL extension for accumulate-mode graphs: the
// runner receives the prior conversation thread plus the new user prompt, so the
// agent's stable system + growing message prefix is reused by the provider's
// prompt cache across sequential nodes. Runners that do not implement it fall
// back to the stateless RunAgentNode path even when Accumulate is on.
type ThreadAgentRunner interface {
	RunAgentNodeThread(ctx context.Context, agentID string, thread []Msg, prompt, outputSchema string) (string, error)
}

// NodeEvent reports a node's lifecycle to an Observer so a caller can stream
// progress (e.g. over SSE) as the graph executes. Output is populated on "done".
type NodeEvent struct {
	Phase  string `json:"phase"` // "start" | "done" | "error"
	NodeID string `json:"nodeId"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Index  int    `json:"index"`            // 1-based execution order
	Output string `json:"output,omitempty"` // on "done"
	Error  string `json:"error,omitempty"`  // on "error"
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
	// Input is the rendered prompt actually sent to an agent node (after
	// {{...}} substitution). Empty for non-agent nodes. Small enough to keep in
	// state; the heavier per-node tool/thinking steps live in a sidecar file (see
	// agent.Runtime.writeFlowNodeSteps), keyed by (runID, nodeID).
	// For a branch node it holds the value that was evaluated (st.Last) so the run
	// inspector can show a decision card.
	Input string `json:"input,omitempty"`
	// ThreadLen is how many accumulated-thread messages this agent node saw as
	// prior context BEFORE its own turn (accumulate mode only; 0/omitted otherwise).
	// The full thread lives in State.Thread; State.Thread[:ThreadLen] is exactly the
	// prior context this node ran with — the run inspector renders it without
	// snapshotting the (growing) thread per node.
	ThreadLen int `json:"threadLen,omitempty"`
	// StartMs/EndMs are unix-millisecond execution bounds, populated for parallel
	// children so the run inspector can draw a concurrency timeline (who ran when,
	// which child was the critical path). 0/omitted for sequentially-run nodes,
	// whose ordering is already implicit in the trace order.
	StartMs int64 `json:"startMs,omitempty"`
	EndMs   int64 `json:"endMs,omitempty"`
	At      int64 `json:"at"`
}

// ctxNodeIDKey carries the id of the node currently executing so an AgentRunner
// (which only receives agentID + prompt) can attribute side outputs — e.g. a
// per-node steps sidecar — to the right node without widening the interface.
type ctxNodeIDKey struct{}

// WithNodeID tags ctx with the executing node's id.
func WithNodeID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxNodeIDKey{}, id)
}

// NodeIDFromContext returns the executing node's id set by WithNodeID ("" if none).
func NodeIDFromContext(ctx context.Context) string {
	s, _ := ctx.Value(ctxNodeIDKey{}).(string)
	return s
}

// State is the restart-safe snapshot persisted after every node. A run can be
// resumed from any saved State.
type State struct {
	Current string            `json:"current"` // next node to run ("" = finished)
	Last    string            `json:"last"`    // most recent output (branch routing + {{last}})
	Outputs map[string]string `json:"outputs"`
	Steps   int               `json:"steps"`
	Trace   []TraceEntry      `json:"trace"`
	// Iter is the current loop iteration (0-based) exposed to body nodes as
	// {{iteration}}. Set by the engine before each loop body pass; nested loops
	// share this field, so an inner loop overwrites it for the duration of its run.
	Iter int `json:"iter,omitempty"`
	// WaitingAt is the id of the await-input node the run is durably suspended at
	// ("" when running/terminal). Run returns (state, nil) with this set when it
	// hits an unfed await-input; the caller persists status=waiting. On resume the
	// caller injects the input into Last and re-enters Run with Current == WaitingAt
	// == the await node, which consumes it and advances. See NodeAwaitInput.
	WaitingAt string `json:"waitingAt,omitempty"`
	// Thread is the accumulated conversation for accumulate-mode graphs: each
	// non-Fresh agent node appends its rendered prompt ({user}) and reply
	// ({assistant}). Empty on legacy stateless runs. Persisted so a resumed run
	// keeps its cacheable prefix. A parallel node forks a copy per child and folds
	// the joined output back as a single synthetic turn (see runParallel).
	Thread []Msg `json:"thread,omitempty"`
	// SubflowRun is the child run id the parent is durably suspended inside when a
	// subflow node's child itself hit an await-input (WaitingAt == that subflow
	// node). On resume the parent feeds its delivered input to this child run; the
	// child either completes (parent advances) or suspends again (parent stays put).
	// Empty unless suspended inside a subflow. See NodeSubflow.
	SubflowRun string `json:"subflowRun,omitempty"`
	// Spawned maps a spawn node id to the async child run ids it launched. A join
	// node awaits these, collects their outputs, and clears the entry. Persisted so
	// a restarted run re-joins the same child runs. See NodeSpawn / NodeJoin.
	Spawned map[string][]string `json:"spawned,omitempty"`
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

// notifyError reports a node failure ("error" phase) so a live observer can stop
// a node's spinner and show why it failed, instead of leaving it pending forever.
func (e *Engine) notifyError(node Node, index int, err error) {
	if e.observer == nil {
		return
	}
	title := node.Title
	if title == "" {
		title = node.ID
	}
	e.observer(NodeEvent{Phase: "error", NodeID: node.ID, Type: node.Type, Title: title, Index: index, Error: err.Error()})
}

// runAgentNodeSafe runs one agent node and converts a panic into an error so a
// crashing node (or a tool/provider panic beneath it) ends the flow cleanly
// instead of taking down the whole process. Used by both the sequential and
// parallel paths.
//
// When useThread is true and the runner implements ThreadAgentRunner, the node
// runs with the accumulated conversation (thread) as its prior messages so the
// provider's prompt cache reuses the stable prefix. The caller owns growing the
// thread (sequential appends in place; parallel forks a copy and folds the join),
// so this method never mutates thread. Falls back to the schema/stateless runner
// when useThread is false or the runner lacks ThreadAgentRunner.
func (e *Engine) runAgentNodeSafe(ctx context.Context, node Node, prompt string, thread []Msg, useThread bool) (out string, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("agent node %q panicked: %v", node.ID, p)
		}
	}()
	// Tag the context so the runner can attribute its per-node steps sidecar.
	ctx = WithNodeID(ctx, node.ID)
	if useThread {
		if tr, ok := e.runner.(ThreadAgentRunner); ok {
			return tr.RunAgentNodeThread(ctx, node.AgentID, thread, prompt, node.OutputSchema)
		}
	}
	if node.OutputSchema != "" {
		if sr, ok := e.runner.(SchemaAgentRunner); ok {
			return sr.RunAgentNodeSchema(ctx, node.AgentID, prompt, node.OutputSchema)
		}
	}
	return e.runner.RunAgentNode(ctx, node.AgentID, prompt)
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
			useThread := g.Accumulate && !node.Fresh
			// Prior-context length this node ran with (before it grows the thread).
			threadLenBefore := len(st.Thread)
			out, err := e.runAgentNodeSafe(ctx, node, prompt, st.Thread, useThread)
			if err != nil {
				e.notifyError(node, st.Steps, err)
				return st, fmt.Errorf("node %q (agent): %w", node.ID, err)
			}
			if useThread {
				// Grow the shared thread so the next same-agent node reuses the
				// cached prefix. Fresh/stateless nodes leave the thread untouched.
				st.Thread = append(st.Thread, Msg{Role: "user", Text: prompt}, Msg{Role: "assistant", Text: out})
			}
			st.Outputs[node.ID] = out
			st.Last = out
			st.appendTraceIn(node, out, prompt)
			if useThread {
				// Tag the just-appended trace with the prior-context length so the
				// inspector can show State.Thread[:ThreadLen] as this node's context.
				st.Trace[len(st.Trace)-1].ThreadLen = threadLenBefore
			}
			e.notify("done", node, st.Steps, out)
			st.Current = node.Next

		case NodeBranch:
			next, label := evalBranch(node, st.Last)
			// Record the evaluated value as Input so the inspector can render a
			// decision card (value → which arm matched); Output carries the label.
			st.appendTraceIn(node, "→ "+label, st.Last)
			e.notify("done", node, st.Steps, "→ "+label)
			st.Current = next

		case NodeDelay:
			e.notify("start", node, st.Steps, "")
			if err := sleepCtx(ctx, node.DelayMs); err != nil {
				e.notifyError(node, st.Steps, err)
				return st, fmt.Errorf("node %q (delay): %w", node.ID, err)
			}
			out := fmt.Sprintf("waited %dms", node.DelayMs)
			st.appendTrace(node, out)
			e.notify("done", node, st.Steps, out)
			st.Current = node.Next

		case NodeTransform:
			out := render(node.Template, input, st)
			st.Outputs[node.ID] = out
			st.Last = out
			st.appendTrace(node, out)
			e.notify("done", node, st.Steps, out)
			st.Current = node.Next

		case NodeParallel:
			combined, childTraces, err := e.runParallel(ctx, g, node, input, st)
			if err != nil {
				return st, err
			}
			// Record each child as its own trace entry (before the parent's fold
			// entry) so the run inspector can open a child's chat-like view; the
			// engine's ctx already tagged each child's steps sidecar by child id.
			st.Trace = append(st.Trace, childTraces...)
			if g.Accumulate {
				// Fold the fan-out back into the parent thread as ONE synthetic
				// user/assistant pair so the thread stays linear, alternating, and
				// ends in assistant (the invariant the next agent node's user turn
				// relies on). The branches' own forked threads are discarded.
				st.Thread = append(st.Thread,
					Msg{Role: "user", Text: parallelFoldMarker(node)},
					Msg{Role: "assistant", Text: combined})
			}
			st.Last = combined
			st.appendTrace(node, combined)
			st.Current = node.JoinNext

		case NodeLoop:
			e.notify("start", node, st.Steps, "")
			next, loopedState, err := e.runLoop(ctx, g, node, input, st, save)
			if err != nil {
				e.notifyError(node, st.Steps, err)
				return loopedState, fmt.Errorf("node %q (loop): %w", node.ID, err)
			}
			st = loopedState
			// A suspend inside the loop body propagates up: Current already points at
			// the await node, so return without advancing to LoopNext.
			if st.WaitingAt != "" {
				return st, nil
			}
			st.appendTrace(node, st.Last)
			e.notify("done", node, st.Steps, st.Last)
			st.Current = next

		case NodeStart:
			// Entry marker: pass straight through to the first real node.
			st.appendTrace(node, "")
			st.Current = node.Next

		case NodeEnd:
			// Optional terminal. Shape the final output (Template) and/or validate it
			// against OutputSchema, then finish.
			if strings.TrimSpace(node.Template) != "" {
				st.Last = render(node.Template, input, st)
				st.Outputs[node.ID] = st.Last
			}
			if s := strings.TrimSpace(node.OutputSchema); s != "" {
				if !json.Valid([]byte(strings.TrimSpace(st.Last))) {
					err := fmt.Errorf("node %q (end): final output does not satisfy the required format (not valid JSON)", node.ID)
					e.notifyError(node, st.Steps, err)
					return st, err
				}
			}
			st.appendTrace(node, st.Last)
			e.notify("done", node, st.Steps, st.Last)
			st.Current = "" // terminal

		case NodeSubflow:
			// advance records the child output and continues past the subflow node.
			advance := func(out string) {
				st.WaitingAt = ""
				st.SubflowRun = ""
				st.Outputs[node.ID] = out
				st.Last = out
				st.appendTrace(node, out)
				e.notify("done", node, st.Steps, out)
				st.Current = node.Next
			}
			// suspend durably parks the parent inside the (still-waiting) child run.
			suspend := func(childRunID string) (State, error) {
				st.WaitingAt = node.ID
				st.SubflowRun = childRunID
				e.notify("waiting", node, st.Steps, "")
				if save != nil {
					if err := save(st); err != nil {
						return st, fmt.Errorf("persist waiting state: %w", err)
					}
				}
				return st, nil
			}

			if st.WaitingAt == node.ID {
				// Resume: feed the parent's delivered input (st.Last) to the child that
				// suspended. Requires the suspendable runner (the same one that parked us).
				sr, ok := e.runner.(SuspendableChildFlowRunner)
				if !ok {
					err := fmt.Errorf("node %q (subflow): runner cannot resume a suspended child", node.ID)
					e.notifyError(node, st.Steps, err)
					return st, err
				}
				out, waiting, err := sr.ResumeChildFlow(ctx, st.SubflowRun, st.Last)
				if err != nil {
					e.notifyError(node, st.Steps, err)
					return st, fmt.Errorf("node %q (subflow): %w", node.ID, err)
				}
				if waiting {
					return suspend(st.SubflowRun) // child suspended again — stay parked
				}
				advance(out)
				break
			}

			e.notify("start", node, st.Steps, "")
			tmpl := node.Template
			if strings.TrimSpace(tmpl) == "" {
				tmpl = "{{last}}"
			}
			childInput := render(tmpl, input, st)
			// Prefer the suspendable runner so a child await-input propagates up; fall
			// back to the plain runner (fail-on-suspend) when unavailable.
			if sr, ok := e.runner.(SuspendableChildFlowRunner); ok {
				out, childRunID, waiting, err := sr.RunChildFlowResumable(ctx, node.FlowRef, childInput)
				if err != nil {
					e.notifyError(node, st.Steps, err)
					return st, fmt.Errorf("node %q (subflow): %w", node.ID, err)
				}
				if waiting {
					return suspend(childRunID)
				}
				advance(out)
				break
			}
			cr, ok := e.runner.(ChildFlowRunner)
			if !ok {
				err := fmt.Errorf("node %q (subflow): runner does not support child flows", node.ID)
				e.notifyError(node, st.Steps, err)
				return st, err
			}
			out, err := cr.RunChildFlow(ctx, node.FlowRef, childInput)
			if err != nil {
				e.notifyError(node, st.Steps, err)
				return st, fmt.Errorf("node %q (subflow): %w", node.ID, err)
			}
			advance(out)

		case NodeSpawn:
			e.notify("start", node, st.Steps, "")
			ar, ok := e.runner.(AsyncFlowRunner)
			if !ok {
				err := fmt.Errorf("node %q (spawn): runner does not support async child flows", node.ID)
				e.notifyError(node, st.Steps, err)
				return st, err
			}
			tmpl := node.Template
			if strings.TrimSpace(tmpl) == "" {
				tmpl = "{{last}}"
			}
			childInput := render(tmpl, input, st)
			runIDs, err := ar.SpawnChildFlows(ctx, node.SpawnFlows, childInput)
			if err != nil {
				e.notifyError(node, st.Steps, err)
				return st, fmt.Errorf("node %q (spawn): %w", node.ID, err)
			}
			if st.Spawned == nil {
				st.Spawned = map[string][]string{}
			}
			st.Spawned[node.ID] = runIDs
			out := fmt.Sprintf("spawned %d flow(s)", len(runIDs))
			st.appendTrace(node, out)
			e.notify("done", node, st.Steps, out)
			st.Current = node.Next

		case NodeJoin:
			e.notify("start", node, st.Steps, "")
			ar, ok := e.runner.(AsyncFlowRunner)
			if !ok {
				err := fmt.Errorf("node %q (join): runner does not support async child flows", node.ID)
				e.notifyError(node, st.Steps, err)
				return st, err
			}
			// Which spawned runs to await: a named spawn node, or every outstanding one.
			var runIDs []string
			var refs []string
			if node.SpawnRef != "" {
				runIDs = st.Spawned[node.SpawnRef]
				refs = []string{node.SpawnRef}
			} else {
				for k, ids := range st.Spawned {
					runIDs = append(runIDs, ids...)
					refs = append(refs, k)
				}
			}
			// Live barrier progress: each poll cycle reports how many child runs have
			// finished, streamed to the run viewer as a "progress" node event.
			onProgress := func(done, total int) {
				e.notify("progress", node, st.Steps, fmt.Sprintf("%d/%d", done, total))
			}
			outputs, err := ar.JoinChildFlows(ctx, runIDs, node.JoinTimeoutSec, node.JoinPartial, onProgress)
			if err != nil {
				e.notifyError(node, st.Steps, err)
				return st, fmt.Errorf("node %q (join): %w", node.ID, err)
			}
			// Clear the joined spawn entries so a re-run of the same spawn/join pair
			// (e.g. inside a loop) starts clean.
			for _, k := range refs {
				delete(st.Spawned, k)
			}
			combined := strings.TrimSpace(strings.Join(outputs, "\n\n"))
			if g.Accumulate {
				st.Thread = append(st.Thread,
					Msg{Role: "user", Text: fmt.Sprintf("[join %s]", node.ID)},
					Msg{Role: "assistant", Text: combined})
			}
			st.Outputs[node.ID] = combined
			st.Last = combined
			st.appendTrace(node, combined)
			e.notify("done", node, st.Steps, combined)
			st.Current = node.Next

		case NodeAwaitInput:
			if st.WaitingAt == node.ID {
				// Resume: input was injected into Last; consume it, clear the wait,
				// and advance. The received input rides {{last}} into the next node
				// (in accumulate mode the next agent's prompt turns it into a user turn).
				st.WaitingAt = ""
				st.Outputs[node.ID] = st.Last
				st.appendTrace(node, st.Last)
				e.notify("done", node, st.Steps, st.Last)
				st.Current = node.Next
			} else {
				// First arrival: durably suspend. Leave Current at this node and set
				// WaitingAt so the caller marks the run waiting and persists state.
				st.WaitingAt = node.ID
				e.notify("waiting", node, st.Steps, "")
				if save != nil {
					if err := save(st); err != nil {
						return st, fmt.Errorf("persist waiting state: %w", err)
					}
				}
				return st, nil
			}

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
// outputs. Each child receives the same incoming value (st.Last) as {{last}}. It
// returns the joined output plus one TraceEntry per child (input+output), so the
// run inspector can open each child's own chat-like view — appended by the caller
// since st is a value copy here (a slice append would not reach the caller).
func (e *Engine) runParallel(ctx context.Context, g Graph, node Node, input string, st State) (string, []TraceEntry, error) {
	type res struct {
		id, title, in, out string
		typ                string
		startMs, endMs     int64
		err                error
	}
	results := make([]res, len(node.Parallel))
	var wg sync.WaitGroup

	for i, childID := range node.Parallel {
		child, ok := g.node(childID)
		if !ok {
			return "", nil, fmt.Errorf("parallel child %q not found", childID)
		}
		e.notify("start", child, st.Steps, "")
		wg.Add(1)
		go func(i int, child Node) {
			defer wg.Done()
			startMs := time.Now().UnixMilli()
			title := child.Title
			if title == "" {
				title = child.ID
			}
			// runAgentNodeSafe converts a panicking child into an error (every flow
			// runs in its own goroutine, so an unrecovered panic here would crash
			// all workspaces). On failure emit an "error" event so the child's live
			// spinner stops instead of hanging pending forever.
			prompt := render(child.Prompt, input, st)
			// Copy-on-fork: each child reads the SAME accumulated prefix (st.Thread)
			// as prior context — same-agent branches share the cached prefix — but a
			// child never grows the parent thread; the join is folded once in Run.
			useChild := g.Accumulate && !child.Fresh
			out, err := e.runAgentNodeSafe(ctx, child, prompt, st.Thread, useChild)
			endMs := time.Now().UnixMilli()
			if err != nil {
				e.notifyError(child, st.Steps, err)
			} else {
				e.notify("done", child, st.Steps, out)
			}
			results[i] = res{id: child.ID, title: title, in: prompt, out: out, typ: child.Type, startMs: startMs, endMs: endMs, err: err}
		}(i, child)
	}
	wg.Wait()

	var b strings.Builder
	traces := make([]TraceEntry, 0, len(results))
	for i := range results {
		r := results[i]
		if r.err != nil {
			return "", nil, fmt.Errorf("parallel child %q: %w", r.id, r.err)
		}
		st.Outputs[r.id] = r.out
		fmt.Fprintf(&b, "[%s]\n%s\n\n", r.title, r.out)
		traces = append(traces, TraceEntry{
			NodeID:  r.id,
			Type:    r.typ,
			Title:   r.title,
			Output:  r.out,
			Input:   r.in,
			StartMs: r.startMs,
			EndMs:   r.endMs,
			At:      time.Now().Unix(),
		})
	}
	return strings.TrimSpace(b.String()), traces, nil
}

// runLoop repeats the loop node's Body sub-chain until an exit condition holds,
// then returns LoopNext. Each pass runs the body via the engine's own driver
// (e.Run) with st.Current set to Body, so the body may contain any node types
// (including nested parallel/loop) and the global maxSteps in Run bounds total
// work across every iteration. Body chains must terminate (Next="") to hand
// control back for the condition check.
//
// Resume limitation: loop control lives on the call stack, not in State, so a
// crash mid-iteration resumes the current body pass and then exits at the body's
// terminal without running the remaining iterations — acceptable for now.
func (e *Engine) runLoop(ctx context.Context, g Graph, node Node, input string, st State, save SaveFunc) (string, State, error) {
	for iter := 0; node.MaxIters <= 0 || iter < node.MaxIters; iter++ {
		st.Iter = iter
		st.Current = node.Body
		var err error
		st, err = e.Run(ctx, g, input, st, save)
		if err != nil {
			return node.LoopNext, st, err
		}
		// Body suspended at an await-input: propagate the wait up (Current points at
		// the await node). NOTE: on resume the loop does not continue iterating — the
		// body remainder runs once and exits (the documented loop-resume limitation).
		if st.WaitingAt != "" {
			return "", st, nil
		}
		if node.Until != "" && loopMatches(node, st.Last) {
			break
		}
	}
	return node.LoopNext, st, nil
}

// loopMatches reports whether the loop's Until condition holds for the value,
// reusing the branch match modes (contains/equals/regex).
func loopMatches(node Node, value string) bool {
	lower := strings.ToLower(value)
	trimmed := strings.ToLower(strings.TrimSpace(value))
	return branchArmMatches(node.UntilMode, node.Until, value, lower, trimmed)
}

// parallelFoldMarker builds the synthetic user turn that precedes a folded
// parallel result in an accumulate-mode thread, naming the fan-out so the
// conversation reads coherently (e.g. "⚡ parallel: Pro, Con → results").
func parallelFoldMarker(node Node) string {
	title := node.Title
	if title == "" {
		title = node.ID
	}
	return fmt.Sprintf("⚡ parallel step %q → results", title)
}

// appendTrace records a node execution (no input; non-agent nodes).
func (st *State) appendTrace(node Node, output string) {
	st.appendTraceIn(node, output, "")
}

// appendTraceIn records a node execution together with the rendered input that
// produced it (agent nodes), so the run inspector can show input→output as a
// chat-like exchange.
func (st *State) appendTraceIn(node Node, output, input string) {
	title := node.Title
	if title == "" {
		title = node.ID
	}
	st.Trace = append(st.Trace, TraceEntry{
		NodeID: node.ID,
		Type:   node.Type,
		Title:  title,
		Output: output,
		Input:  input,
		At:     time.Now().Unix(),
	})
}

// evalBranch routes by matching the incoming value against each arm's Contains
// using node.MatchMode. An empty Contains is the default arm, taken only when no
// other arm matches (evaluated regardless of its position). Returns (nextID, label).
//
//	contains (default) — case-insensitive substring
//	equals             — case-insensitive, trimmed exact match
//	regex              — Go regexp on the raw value (invalid patterns never match)
func evalBranch(node Node, value string) (string, string) {
	// Structured routing: extract the named top-level JSON field as the matched
	// value (schema-constrained upstream nodes make this parse-proof). Falls
	// back to the raw text when the output isn't JSON or lacks the field.
	if node.JSONField != "" {
		if v, ok := jsonTopField(value, node.JSONField); ok {
			value = v
		}
	}
	lower := strings.ToLower(value)
	trimmed := strings.ToLower(strings.TrimSpace(value))
	var def *Branch
	for i := range node.Branches {
		b := node.Branches[i]
		if b.Contains == "" {
			def = &node.Branches[i]
			continue
		}
		if branchArmMatches(node.MatchMode, b.Contains, value, lower, trimmed) {
			return b.Next, b.Contains
		}
	}
	if def != nil {
		return def.Next, "default"
	}
	return "", "no match"
}

// jsonTopField extracts a top-level field from a JSON object output, rendered
// as a plain string (strings verbatim; numbers/bools via fmt).
func jsonTopField(raw, field string) (string, bool) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &obj); err != nil {
		return "", false
	}
	v, ok := obj[field]
	if !ok {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case nil:
		return "", false
	default:
		return fmt.Sprintf("%v", t), true
	}
}

// branchArmMatches reports whether one arm's pattern matches the value under the
// given match mode.
func branchArmMatches(mode, pattern, raw, lower, trimmed string) bool {
	switch mode {
	case "equals":
		return trimmed == strings.ToLower(strings.TrimSpace(pattern))
	case "regex":
		ok, err := regexp.MatchString(pattern, raw)
		return err == nil && ok
	default: // "" or "contains"
		return strings.Contains(lower, strings.ToLower(pattern))
	}
}

// sleepCtx waits ms milliseconds or until the context is cancelled. The wait is
// capped so a misconfigured delay can't block a run indefinitely.
func sleepCtx(ctx context.Context, ms int) error {
	if ms <= 0 {
		return nil
	}
	if ms > maxDelayMs {
		ms = maxDelayMs
	}
	t := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// render substitutes template placeholders in a prompt:
//
//	{{input}}        the flow's input
//	{{last}}         the most recent node output
//	{{node.<id>}}    a specific node's stored output
//	{{date}}         current date (2006-01-02)
//	{{time}}         current time (15:04)
//	{{datetime}}     current date + time (2006-01-02 15:04)
//	{{iteration}}    the current loop iteration (0-based); 0 outside a loop
//
// The date/time placeholders resolve to the wall-clock at render time (mirrors the
// automation engine's turnVars format). On a resumed run they reflect the resume
// moment, not the original start — acceptable for these cosmetic "now" values.
func render(tmpl, input string, st State) string {
	out := strings.ReplaceAll(tmpl, "{{input}}", input)
	out = strings.ReplaceAll(out, "{{last}}", st.Last)
	for id, v := range st.Outputs {
		out = strings.ReplaceAll(out, "{{node."+id+"}}", v)
	}
	now := time.Now()
	out = strings.ReplaceAll(out, "{{date}}", now.Format("2006-01-02"))
	out = strings.ReplaceAll(out, "{{time}}", now.Format("15:04"))
	out = strings.ReplaceAll(out, "{{datetime}}", now.Format("2006-01-02 15:04"))
	out = strings.ReplaceAll(out, "{{iteration}}", strconv.Itoa(st.Iter))
	return out
}
