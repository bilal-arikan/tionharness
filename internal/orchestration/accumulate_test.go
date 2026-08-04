package orchestration

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// threadRunner records the thread it was handed on each call and echoes the
// prompt. It implements BOTH RunAgentNode (stateless fallback) and
// RunAgentNodeThread (accumulate path) so tests can assert which path ran and
// what prior context the node saw. A parallel node fans its children out across
// goroutines, so the recording fields are mutex-guarded; the post-Run assertions
// read them only after Run has joined every child.
type threadRunner struct {
	mu         sync.Mutex
	threadLens []int    // len(thread) captured at each RunAgentNodeThread call
	threads    [][]Msg  // the thread slice seen at each accumulate call
	statelessN int      // times the stateless path ran
	calls      []string // prompts seen, in order (both paths)
}

func (r *threadRunner) RunAgentNode(_ context.Context, _ string, prompt string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statelessN++
	r.calls = append(r.calls, prompt)
	return "out:" + prompt, nil
}

func (r *threadRunner) RunAgentNodeThread(_ context.Context, _ string, thread []Msg, prompt, _ string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.threadLens = append(r.threadLens, len(thread))
	cp := append([]Msg(nil), thread...)
	r.threads = append(r.threads, cp)
	r.calls = append(r.calls, prompt)
	return "out:" + prompt, nil
}

// chainGraph is two sequential agent nodes a → b.
func chainGraph(accumulate bool) Graph {
	return Graph{
		Start:      "a",
		Accumulate: accumulate,
		Nodes: []Node{
			{ID: "a", Type: NodeAgent, AgentID: "ag", Prompt: "first {{input}}", Next: "b"},
			{ID: "b", Type: NodeAgent, AgentID: "ag", Prompt: "second", Next: ""},
		},
	}
}

// TestAccumulate_GrowsThreadAcrossNodes verifies that with Accumulate on, the
// second node sees the first node's user+assistant pair as prior context, and
// the final State.Thread holds both turns — the cache-friendly growing prefix.
func TestAccumulate_GrowsThreadAcrossNodes(t *testing.T) {
	r := &threadRunner{}
	eng := NewEngine(r)
	st, err := eng.Run(context.Background(), chainGraph(true), "X", NewState(chainGraph(true)), nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if r.statelessN != 0 {
		t.Errorf("accumulate run should never use the stateless path, got %d calls", r.statelessN)
	}
	if len(r.threadLens) != 2 {
		t.Fatalf("expected 2 accumulate calls, got %d", len(r.threadLens))
	}
	// Node a starts with an empty thread; node b sees a's [user, assistant].
	if r.threadLens[0] != 0 || r.threadLens[1] != 2 {
		t.Errorf("expected thread lengths [0 2], got %v", r.threadLens)
	}
	// The prompt template must be rendered before it enters the thread.
	if got := r.threads[1][0].Text; got != "first X" {
		t.Errorf("node b's prior user turn should be the rendered prompt %q, got %q", "first X", got)
	}
	if got := r.threads[1][1].Text; got != "out:first X" {
		t.Errorf("node b's prior assistant turn should be a's reply, got %q", got)
	}
	// Final thread = both turns fully accumulated.
	if len(st.Thread) != 4 {
		t.Errorf("final thread should hold 2 pairs (4 msgs), got %d", len(st.Thread))
	}
}

// TestAccumulate_Off_UsesStatelessPath verifies the default (Accumulate off) never
// threads — every node runs as an isolated single-message call (legacy behavior).
func TestAccumulate_Off_UsesStatelessPath(t *testing.T) {
	r := &threadRunner{}
	eng := NewEngine(r)
	g := chainGraph(false)
	st, err := eng.Run(context.Background(), g, "X", NewState(g), nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(r.threadLens) != 0 {
		t.Errorf("stateless run should never thread, got %d accumulate calls", len(r.threadLens))
	}
	if r.statelessN != 2 {
		t.Errorf("expected 2 stateless calls, got %d", r.statelessN)
	}
	if len(st.Thread) != 0 {
		t.Errorf("stateless run should leave the thread empty, got %d", len(st.Thread))
	}
}

// TestAccumulate_FreshNodeOptsOut verifies a Fresh node in an accumulate graph runs
// stateless and does not grow the thread, while its non-Fresh neighbors do.
func TestAccumulate_FreshNodeOptsOut(t *testing.T) {
	r := &threadRunner{}
	g := Graph{
		Start:      "a",
		Accumulate: true,
		Nodes: []Node{
			{ID: "a", Type: NodeAgent, AgentID: "ag", Prompt: "first", Next: "b"},
			{ID: "b", Type: NodeAgent, AgentID: "ag", Prompt: "isolated", Next: "c", Fresh: true},
			{ID: "c", Type: NodeAgent, AgentID: "ag", Prompt: "third", Next: ""},
		},
	}
	eng := NewEngine(r)
	st, err := eng.Run(context.Background(), g, "X", NewState(g), nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	// a + c threaded (2 accumulate calls); b ran stateless (1 call).
	if len(r.threadLens) != 2 || r.statelessN != 1 {
		t.Fatalf("expected 2 threaded + 1 stateless, got %d threaded / %d stateless", len(r.threadLens), r.statelessN)
	}
	// c sees only a's pair (2) — b's isolated turn never entered the thread.
	if r.threadLens[1] != 2 {
		t.Errorf("node c should see only a's pair (len 2), got %d", r.threadLens[1])
	}
	if len(st.Thread) != 4 {
		t.Errorf("final thread should hold a's + c's pairs (4 msgs), got %d", len(st.Thread))
	}
}

// TestAccumulate_ParallelFold verifies a parallel node folds its fan-out into the
// thread as ONE synthetic user/assistant pair (keeping it alternating + ending in
// assistant), and each child saw the pre-parallel prefix.
func TestAccumulate_ParallelFold(t *testing.T) {
	r := &threadRunner{}
	g := Graph{
		Start:      "a",
		Accumulate: true,
		Nodes: []Node{
			{ID: "a", Type: NodeAgent, AgentID: "ag", Prompt: "seed", Next: "p"},
			{ID: "p", Type: NodeParallel, Parallel: []string{"c1", "c2"}, JoinNext: ""},
			{ID: "c1", Type: NodeAgent, AgentID: "ag", Prompt: "left"},
			{ID: "c2", Type: NodeAgent, AgentID: "ag", Prompt: "right"},
		},
	}
	eng := NewEngine(r)
	st, err := eng.Run(context.Background(), g, "X", NewState(g), nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	// a (2) + fold pair (2) = 4 messages.
	if len(st.Thread) != 4 {
		t.Fatalf("expected thread of 4 (a pair + fold pair), got %d: %+v", len(st.Thread), st.Thread)
	}
	// The fold's user marker precedes the assistant-combined output, and the
	// thread must end in assistant so a downstream agent's user turn alternates.
	if st.Thread[2].Role != "user" || !strings.Contains(st.Thread[2].Text, "parallel") {
		t.Errorf("fold[0] should be a user marker naming the parallel, got %+v", st.Thread[2])
	}
	if st.Thread[3].Role != "assistant" {
		t.Errorf("thread must end in assistant, got role %q", st.Thread[3].Role)
	}
	// Both children ran through the accumulate path seeing a's pair (len 2), never
	// growing the parent individually.
	if len(r.threadLens) != 3 { // a + c1 + c2
		t.Fatalf("expected 3 accumulate calls (a, c1, c2), got %d", len(r.threadLens))
	}
	if r.threadLens[1] != 2 || r.threadLens[2] != 2 {
		t.Errorf("both children should fork a's prefix (len 2), got %v", r.threadLens[1:])
	}
}
