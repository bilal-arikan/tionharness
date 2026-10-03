package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// scriptRunner answers llm nodes from a script keyed by "node#visit" (or
// "node"), judges by exact option match, checks criteria by substring
// ("criterion text" must appear in the value) and records trigger payloads.
type scriptRunner struct {
	replies  map[string]string
	calls    []string
	fail     string
	checkErr error
	fired    []string
}

func (s *scriptRunner) Check(_ context.Context, _ Node, value string, criteria []string) ([]bool, error) {
	if s.checkErr != nil {
		return nil, s.checkErr
	}
	out := make([]bool, len(criteria))
	for i, c := range criteria {
		out[i] = strings.Contains(strings.ToLower(value), strings.ToLower(c))
	}
	return out, nil
}

func (s *scriptRunner) Trigger(_ context.Context, n Node, payload string) (string, error) {
	if s.fail == n.ID {
		return "", errors.New("automation gone")
	}
	s.fired = append(s.fired, n.AutomationID+":"+payload)
	return "fired " + n.AutomationID, nil
}

func (s *scriptRunner) RunLLM(_ context.Context, n Node, prompt string, visit int) (string, error) {
	s.calls = append(s.calls, n.ID+":"+prompt)
	if s.fail == n.ID {
		return "", errors.New("boom")
	}
	if r, ok := s.replies[fmt.Sprintf("%s#%d", n.ID, visit)]; ok {
		return r, nil
	}
	if r, ok := s.replies[n.ID]; ok {
		return r, nil
	}
	return "reply:" + prompt, nil
}

func (s *scriptRunner) Judge(_ context.Context, _ Node, value string, options []string) (int, error) {
	for i, o := range options {
		if strings.EqualFold(strings.TrimSpace(value), o) {
			return i, nil
		}
	}
	return -1, nil
}

func TestDefaultGraphValidatesAndIsTrivial(t *testing.T) {
	g := DefaultGraph()
	if err := g.Validate(); err != nil {
		t.Fatalf("default graph invalid: %v", err)
	}
	if !g.IsTrivial() {
		t.Fatal("default graph must be trivial")
	}
	rt := &scriptRunner{}
	res := Run(context.Background(), g, "merhaba", rt, nil)
	if res.Err != nil {
		t.Fatalf("run: %v", res.Err)
	}
	if res.Output != "reply:merhaba" {
		t.Fatalf("output = %q", res.Output)
	}
	if len(res.Steps) != 3 {
		t.Fatalf("steps = %d", len(res.Steps))
	}
	// Round-trip through JSON keeps it trivial.
	back, err := Parse(Encode(g))
	if err != nil || !back.IsTrivial() {
		t.Fatalf("round trip: %v trivial=%v", err, back.IsTrivial())
	}
}

func critiqueGraph() Graph {
	return Graph{Nodes: []Node{
		{ID: "input", Type: NodeInput},
		{ID: "draft", Type: NodeLLM, Prompt: "Answer: {{input}}"},
		{ID: "critic", Type: NodeLLM, Context: ContextFresh, Tools: ToolsNone, Prompt: "Review {{node.draft}} (visit {{visit}})"},
		{ID: "check", Type: NodeRoute, Mode: ModeContains, MaxVisits: 2},
		{ID: "output", Type: NodeOutput, Template: "{{node.draft}}"},
	}, Edges: []Edge{
		{From: "input", To: "draft"},
		{From: "draft", To: "critic"},
		{From: "critic", To: "check"},
		{From: "check", To: "output", When: "APPROVE"},
		{From: "check", To: "draft"},
	}}
}

func TestFeedbackLoopExitsOnApprove(t *testing.T) {
	g := critiqueGraph()
	if err := g.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if g.IsTrivial() {
		t.Fatal("loop graph must not be trivial")
	}
	rt := &scriptRunner{replies: map[string]string{
		"draft#1": "v1", "draft#2": "v2",
		"critic#1": "REVISE: weak", "critic#2": "APPROVE",
	}}
	var events []Event
	res := Run(context.Background(), g, "q", rt, func(e Event) { events = append(events, e) })
	if res.Err != nil {
		t.Fatalf("run: %v", res.Err)
	}
	if res.Output != "v2" {
		t.Fatalf("output = %q, steps=%+v", res.Output, res.Steps)
	}
	// input, draft, critic, check(default→draft), draft, critic, check(APPROVE), output
	if len(res.Steps) != 8 {
		t.Fatalf("steps = %d", len(res.Steps))
	}
	if res.Steps[3].Edge != "*" || res.Steps[6].Edge != "APPROVE" {
		t.Fatalf("edges: %q %q", res.Steps[3].Edge, res.Steps[6].Edge)
	}
	if !strings.Contains(rt.calls[1], "Review v1 (visit 1)") {
		t.Fatalf("critic prompt not rendered: %q", rt.calls[1])
	}
	if len(events) != 16 {
		t.Fatalf("events = %d", len(events))
	}
}

func TestVisitCapForcesDefaultArm(t *testing.T) {
	g := critiqueGraph()
	// Critic never approves: the route's visit cap (2) forces the default arm
	// on the third visit, which loops to draft... the cap applies to the route
	// node itself, so after 2 loop-backs the default arm is forced. Here the
	// default arm IS the loop-back, so a graph like this would spin until the
	// step cap; make the default arm the exit instead.
	g.Edges = []Edge{
		{From: "input", To: "draft"},
		{From: "draft", To: "critic"},
		{From: "critic", To: "check"},
		{From: "check", To: "draft", When: "REVISE"},
		{From: "check", To: "output"},
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	rt := &scriptRunner{replies: map[string]string{"critic": "REVISE", "draft": "d"}}
	res := Run(context.Background(), g, "q", rt, nil)
	if res.Err != nil {
		t.Fatalf("run: %v", res.Err)
	}
	// check visited 3 times: REVISE, REVISE, then cap → default (output).
	visits := 0
	last := ""
	for _, s := range res.Steps {
		if s.NodeID == "check" {
			visits++
			last = s.Edge
		}
	}
	if visits != 3 || !strings.HasPrefix(last, "*") {
		t.Fatalf("visits=%d last=%q", visits, last)
	}
}

func TestStepCapStopsRunawayLoop(t *testing.T) {
	g := critiqueGraph()
	g.MaxSteps = 5
	// default arm loops back forever (critic never approves)
	rt := &scriptRunner{replies: map[string]string{"critic": "REVISE", "draft": "d"}}
	res := Run(context.Background(), g, "q", rt, nil)
	if !errors.Is(res.Err, ErrStepCap) {
		t.Fatalf("expected step cap, got %v", res.Err)
	}
	if len(res.Steps) != 5 {
		t.Fatalf("partial trace = %d", len(res.Steps))
	}
}

func TestValidateRejectsBrokenGraphs(t *testing.T) {
	cases := map[string]func(g *Graph){
		"two inputs": func(g *Graph) { g.Nodes = append(g.Nodes, Node{ID: "in2", Type: NodeInput}) },
		"no output":  func(g *Graph) { g.Nodes = g.Nodes[:2]; g.Edges = g.Edges[:1] },
		"dangling":   func(g *Graph) { g.Edges = append(g.Edges, Edge{From: "respond", To: "ghost"}) },
		"unreachable": func(g *Graph) {
			g.Nodes = append(g.Nodes, Node{ID: "lost", Type: NodeTransform, Template: "x"})
			g.Edges = append(g.Edges, Edge{From: "lost", To: "output"})
		},
		"dead end": func(g *Graph) {
			g.Nodes = append(g.Nodes, Node{ID: "dead", Type: NodeTransform, Template: "x"})
			g.Edges = append(g.Edges, Edge{From: "input", To: "dead"})
		},
		"loop without exit": func(g *Graph) {
			g.Nodes = append(g.Nodes, Node{ID: "again", Type: NodeTransform, Template: "{{last}}"})
			g.Edges = []Edge{{From: "input", To: "respond"}, {From: "respond", To: "again"}, {From: "again", To: "respond"}}
			g.Nodes = append(g.Nodes, Node{ID: "r", Type: NodeRoute})
			g.Edges = append(g.Edges, Edge{From: "r", To: "output"})
			g.Edges = append(g.Edges, Edge{From: "input", To: "r"})
		},
		"unknown template ref": func(g *Graph) { g.Nodes[1].Prompt = "{{node.nope}}" },
		"bad regexp": func(g *Graph) {
			g.Nodes = append(g.Nodes, Node{ID: "r", Type: NodeRoute, Mode: ModeRegex})
			g.Edges = []Edge{{From: "input", To: "respond"}, {From: "respond", To: "r"}, {From: "r", To: "output", When: "("}, {From: "r", To: "output"}}
		},
	}
	for name, mutate := range cases {
		g := DefaultGraph()
		mutate(&g)
		if err := g.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestApplyOpsInsertCriticLoop(t *testing.T) {
	g := DefaultGraph()
	ops := []Op{
		{Op: "insert_between", From: "respond", To: "output", Node: &Node{ID: "critic", Type: NodeLLM, Context: ContextFresh, Tools: ToolsNone, Prompt: "Review: {{node.respond}}. Reply APPROVE or REVISE."}},
		{Op: "insert_between", From: "critic", To: "output", Node: &Node{ID: "check", Type: NodeRoute}},
		{Op: "update_edge", ID: "e_check_output", Edge: &Edge{When: "APPROVE"}},
		{Op: "add_edge", Edge: &Edge{From: "check", To: "respond"}},
		{Op: "update_node", ID: "respond", Fields: json.RawMessage(`{"prompt":"{{input}}\n\nFeedback: {{node.critic}}","title":"Yanıt v2"}`)},
	}
	next, err := Apply(g, ops)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(next.Nodes) != 5 || len(next.Edges) != 5 {
		t.Fatalf("nodes=%d edges=%d", len(next.Nodes), len(next.Edges))
	}
	d := Diff(g, next)
	if len(d.AddedNodes) != 2 || len(d.ChangedNodes) != 1 {
		t.Fatalf("diff = %+v", d)
	}
	// Removing critic bridges respond→check.
	removed, err := Apply(next, []Op{
		{Op: "update_node", ID: "respond", Fields: json.RawMessage(`{"prompt":"{{input}}"}`)},
		{Op: "remove_node", ID: "critic"},
	})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !removed.hasEdge("respond", "check") {
		t.Fatalf("bridge missing: %+v", removed.Edges)
	}
	// A dangling {{node.critic}} reference is caught by validation.
	if _, err := Apply(next, []Op{{Op: "remove_node", ID: "critic"}}); err == nil {
		t.Fatal("dangling template reference must fail validation")
	}
	// Immutable fields and protected nodes.
	if _, err := Apply(g, []Op{{Op: "update_node", ID: "respond", Fields: json.RawMessage(`{"type":"route"}`)}}); err == nil {
		t.Fatal("type must be immutable")
	}
	if _, err := Apply(g, []Op{{Op: "remove_node", ID: "output"}}); err == nil {
		t.Fatal("output must not be removable")
	}
}

func TestJSONRouteAndJudge(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: "input", Type: NodeInput},
		{ID: "classify", Type: NodeLLM},
		{ID: "route", Type: NodeRoute, Mode: ModeJSON, JSONField: "kind"},
		{ID: "a", Type: NodeTransform, Template: "A:{{input}}"},
		{ID: "b", Type: NodeTransform, Template: "B:{{input}}"},
		{ID: "output", Type: NodeOutput},
	}, Edges: []Edge{
		{From: "input", To: "classify"}, {From: "classify", To: "route"},
		{From: "route", To: "a", When: "question"}, {From: "route", To: "b"},
		{From: "a", To: "output"}, {From: "b", To: "output"},
	}}
	if err := g.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	rt := &scriptRunner{replies: map[string]string{"classify": "```json\n{\"kind\":\"question\"}\n```"}}
	res := Run(context.Background(), g, "x", rt, nil)
	if res.Err != nil || res.Output != "A:x" {
		t.Fatalf("json route: %v %q", res.Err, res.Output)
	}
	g.Nodes[2] = Node{ID: "route", Type: NodeRoute, Mode: ModeJudge}
	rt = &scriptRunner{replies: map[string]string{"classify": "question"}}
	res = Run(context.Background(), g, "x", rt, nil)
	if res.Err != nil || res.Output != "A:x" {
		t.Fatalf("judge route: %v %q", res.Err, res.Output)
	}
	rt = &scriptRunner{replies: map[string]string{"classify": "unknown"}}
	res = Run(context.Background(), g, "x", rt, nil)
	if res.Err != nil || res.Output != "B:x" {
		t.Fatalf("judge default: %v %q", res.Err, res.Output)
	}
}

func TestLLMErrorKeepsPartialTrace(t *testing.T) {
	g := critiqueGraph()
	rt := &scriptRunner{fail: "critic"}
	res := Run(context.Background(), g, "q", rt, nil)
	if res.Err == nil || !strings.Contains(res.Err.Error(), `node "critic"`) {
		t.Fatalf("err = %v", res.Err)
	}
	if len(res.Steps) != 3 || res.Steps[2].Error == "" {
		t.Fatalf("steps = %+v", res.Steps)
	}
}

func TestRenderPlaceholders(t *testing.T) {
	out := Render("{{input}}|{{last}}|{{node.a}}|{{ node.b }}|{{visit}}|{{nope}}", Vars{Input: "i", Last: "l", Outputs: map[string]string{"a": "A"}, Visit: 2})
	if out != "i|l|A||2|{{nope}}" {
		t.Fatalf("render = %q", out)
	}
}

func TestSummary(t *testing.T) {
	s := critiqueGraph().Normalized().Summary()
	if !strings.HasPrefix(s, "input → draft(llm) → critic(llm) → check(route: APPROVE→output, *→draft)") {
		t.Fatalf("summary = %q", s)
	}
}

func criteriaGraph() Graph {
	return Graph{Nodes: []Node{
		{ID: "input", Type: NodeInput},
		{ID: "draft", Type: NodeLLM, Prompt: "{{input}}"},
		{ID: "gate", Type: NodeRoute, Mode: ModeCriteria, Criteria: []string{"hello", "world"}, MaxVisits: 2},
		{ID: "fix", Type: NodeLLM, Context: ContextFresh, Tools: ToolsNone, Prompt: "Fix: {{node.draft}}"},
		{ID: "output", Type: NodeOutput, Template: "{{last}}"},
	}, Edges: []Edge{
		{From: "input", To: "draft"},
		{From: "draft", To: "gate"},
		{From: "gate", To: "output", When: "pass"},
		{From: "gate", To: "fix", When: "fail"},
		{From: "fix", To: "gate"},
		{ID: "e_gate_default", From: "gate", To: "output"},
	}}
}

func TestCriteriaRoutePassFailAndFallback(t *testing.T) {
	g := criteriaGraph()
	if err := g.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	// All criteria hold → pass arm straight to the output.
	r := &scriptRunner{replies: map[string]string{"draft": "hello world"}}
	res := Run(context.Background(), g, "x", r, nil)
	if res.Err != nil || res.Output != "hello world" {
		t.Fatalf("pass: err=%v out=%q", res.Err, res.Output)
	}
	if st := res.Steps[2]; st.Edge != "pass" || !strings.Contains(st.Detail, "2/2") {
		t.Errorf("pass step = %+v", st)
	}
	// One criterion fails → fail arm → fix → gate again (fix output passes).
	r = &scriptRunner{replies: map[string]string{"draft": "hello there", "fix": "hello world fixed"}}
	res = Run(context.Background(), g, "x", r, nil)
	if res.Err != nil || res.Output != "hello world fixed" {
		t.Fatalf("fail→fix: err=%v out=%q", res.Err, res.Output)
	}
	if st := res.Steps[2]; st.Edge != "fail" || !strings.Contains(st.Detail, "failed: world") {
		t.Errorf("fail step = %+v", st)
	}
	// Checker unavailable → default arm (output) with the reason in the detail.
	r = &scriptRunner{replies: map[string]string{"draft": "nothing"}, checkErr: errors.New("decider off")}
	res = Run(context.Background(), g, "x", r, nil)
	if res.Err != nil || res.Output != "nothing" {
		t.Fatalf("fallback: err=%v out=%q", res.Err, res.Output)
	}
	if st := res.Steps[2]; st.Edge != "*" || !strings.Contains(st.Detail, "decider off") {
		t.Errorf("fallback step = %+v", st)
	}
	// Validation: criteria mode needs criteria and a pass/fail arm.
	bad := criteriaGraph()
	bad.Nodes[2].Criteria = nil
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "criterion") {
		t.Errorf("empty criteria must fail validation, got %v", err)
	}
	bad = criteriaGraph()
	bad.Edges[2].When, bad.Edges[3].When = "yes", "no"
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "pass") {
		t.Errorf("missing pass/fail arms must fail validation, got %v", err)
	}
}

func TestTriggerNodePassesLastThrough(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: "input", Type: NodeInput},
		{ID: "respond", Type: NodeLLM, Prompt: "{{input}}"},
		{ID: "notify", Type: NodeTrigger, AutomationID: "AUT1", Template: "Reply was: {{last}}"},
		{ID: "output", Type: NodeOutput},
	}, Edges: []Edge{
		{From: "input", To: "respond"},
		{From: "respond", To: "notify"},
		{From: "notify", To: "output"},
	}}
	if err := g.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	r := &scriptRunner{replies: map[string]string{"respond": "fine"}}
	res := Run(context.Background(), g, "hi", r, nil)
	if res.Err != nil || res.Output != "fine" {
		t.Fatalf("err=%v out=%q (the trigger must not replace the reply)", res.Err, res.Output)
	}
	if len(r.fired) != 1 || r.fired[0] != "AUT1:Reply was: fine" {
		t.Errorf("fired = %v", r.fired)
	}
	if st := res.Steps[2]; st.Type != NodeTrigger || st.Detail != "fired AUT1" || res.Outputs["notify"] != "fired AUT1" {
		t.Errorf("trigger step = %+v outputs=%v", st, res.Outputs)
	}
	// A failing automation fails the run (the trace keeps the partial steps).
	r = &scriptRunner{fail: "notify"}
	res = Run(context.Background(), g, "hi", r, nil)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "automation gone") || len(res.Steps) != 3 {
		t.Errorf("failure: err=%v steps=%d", res.Err, len(res.Steps))
	}
	// Validation: a trigger needs its automation id.
	g.Nodes[2].AutomationID = ""
	if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "automationId") {
		t.Errorf("missing automationId must fail validation, got %v", err)
	}
}
