package orchestration

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// judgeRunner is an agent runner that also judges: branch picks and loop
// conditions come from the fields below, and every request is recorded.
type judgeRunner struct {
	iterRunner
	pick      int
	conf      float64
	holdsAt   int // JudgeCondition returns true from this call on (1-based); 0 = never
	err       error
	condCalls int
	requests  []JudgeRequest
}

func (r *judgeRunner) JudgeBranch(_ context.Context, req JudgeRequest) (int, float64, error) {
	r.requests = append(r.requests, req)
	return r.pick, r.conf, r.err
}

func (r *judgeRunner) JudgeCondition(_ context.Context, req JudgeRequest) (bool, float64, error) {
	r.requests = append(r.requests, req)
	r.condCalls++
	if r.err != nil {
		return false, 0, r.err
	}
	return r.holdsAt > 0 && r.condCalls >= r.holdsAt, 0.9, nil
}

// judgeBranchGraph: an agent node, then a judge branch with two described arms
// and (optionally) a default arm.
func judgeBranchGraph(withDefault bool) Graph {
	arms := []Branch{
		{Contains: "A bug report: something is broken", Next: "bug"},
		{Contains: "A feature request", Next: "feature"},
	}
	if withDefault {
		arms = append(arms, Branch{Contains: "", Next: "other"})
	}
	return Graph{
		Start: "s",
		Nodes: []Node{
			{ID: "s", Type: NodeStart, Next: "classify"},
			{ID: "classify", Type: NodeAgent, AgentID: "AGT7", Prompt: "{{input}}", Next: "route"},
			{ID: "route", Type: NodeBranch, MatchMode: MatchJudge, JudgeQuestion: "What kind of request is this?", Branches: arms},
			{ID: "bug", Type: NodeTransform, Template: "BUG"},
			{ID: "feature", Type: NodeTransform, Template: "FEATURE"},
			{ID: "other", Type: NodeTransform, Template: "OTHER"},
		},
	}
}

func TestJudgeBranchRoutesToPickedArm(t *testing.T) {
	r := &judgeRunner{pick: 1, conf: 0.93}
	g := judgeBranchGraph(true)
	st, err := NewEngine(r).Run(context.Background(), g, "please add dark mode", NewState(g), nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Last != "FEATURE" {
		t.Errorf("last = %q, want FEATURE", st.Last)
	}
	req := r.requests[0]
	if len(req.Options) != 2 || req.Options[0] != "A bug report: something is broken" || req.AgentID != "AGT7" || req.Question == "" {
		t.Errorf("judge request = %+v", req)
	}
	if req.Value != "out:please add dark mode" {
		t.Errorf("judged value = %q, want the previous node's output", req.Value)
	}
	var label string
	for _, tr := range st.Trace {
		if tr.NodeID == "route" {
			label = tr.Output
		}
	}
	if !strings.Contains(label, "A feature request") || !strings.Contains(label, "judge 0.93") {
		t.Errorf("trace label = %q", label)
	}
}

func TestJudgeBranchUnsureOrFailingTakesDefault(t *testing.T) {
	for name, r := range map[string]*judgeRunner{
		"unsure": {pick: -1, conf: 0.41},
		"error":  {err: errors.New("decider is disabled")},
	} {
		g := judgeBranchGraph(true)
		st, err := NewEngine(r).Run(context.Background(), g, "x", NewState(g), nil)
		if err != nil || st.Last != "OTHER" {
			t.Errorf("%s: last = %q, err = %v; want the default arm", name, st.Last, err)
		}
	}
	// No default arm: an unsure answer ends the flow like "no match", but a
	// judge that cannot answer fails the node instead of silently ending.
	g := judgeBranchGraph(false)
	if st, err := NewEngine(&judgeRunner{pick: -1, conf: 0.3}).Run(context.Background(), g, "x", NewState(g), nil); err != nil || st.Current != "" {
		t.Errorf("unsure without default: err = %v, current = %q", err, st.Current)
	}
	if _, err := NewEngine(&judgeRunner{err: errors.New("down")}).Run(context.Background(), g, "x", NewState(g), nil); err == nil {
		t.Error("a failing judge without a default arm must fail the run")
	}
	// A runner that cannot judge at all behaves like a failing judge.
	g = judgeBranchGraph(true)
	if st, err := NewEngine(okRunner{}).Run(context.Background(), g, "x", NewState(g), nil); err != nil || st.Last != "OTHER" {
		t.Errorf("runner without judge: last = %q, err = %v", st.Last, err)
	}
}

func TestJudgeLoopExitsWhenConditionHolds(t *testing.T) {
	r := &judgeRunner{holdsAt: 2}
	g := loopGraph(10, "The reviewer approved the change", MatchJudge)
	st, err := NewEngine(r).Run(context.Background(), g, "X", NewState(g), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.prompts) != 2 {
		t.Errorf("body passes = %d, want 2 (condition held on the second check)", len(r.prompts))
	}
	if !strings.HasPrefix(st.Last, "done:") {
		t.Errorf("last = %q, want LoopNext to have run", st.Last)
	}
	if c := r.requests[0].Condition; c != "The reviewer approved the change" {
		t.Errorf("condition = %q", c)
	}
}

func TestJudgeLoopErrors(t *testing.T) {
	// With an iteration cap, a failing judge just means "not done yet".
	r := &judgeRunner{err: errors.New("timeout")}
	g := loopGraph(3, "approved", MatchJudge)
	if _, err := NewEngine(r).Run(context.Background(), g, "X", NewState(g), nil); err != nil {
		t.Errorf("capped loop with failing judge: %v", err)
	}
	if len(r.prompts) != 3 {
		t.Errorf("body passes = %d, want the cap (3)", len(r.prompts))
	}
	// Without a cap nothing else would end the loop: fail instead.
	g = loopGraph(0, "approved", MatchJudge)
	if _, err := NewEngine(&judgeRunner{err: errors.New("timeout")}).Run(context.Background(), g, "X", NewState(g), nil); err == nil {
		t.Error("uncapped loop with failing judge must fail")
	}
}

func TestValidateJudgeBranchNeedsOptions(t *testing.T) {
	g := Graph{Start: "s", Nodes: []Node{
		{ID: "s", Type: NodeStart, Next: "b"},
		{ID: "b", Type: NodeBranch, MatchMode: MatchJudge, Branches: []Branch{{Contains: "", Next: ""}}},
	}}
	if err := g.Validate(); err == nil {
		t.Error("a judge branch with only a default arm was accepted")
	}
}
