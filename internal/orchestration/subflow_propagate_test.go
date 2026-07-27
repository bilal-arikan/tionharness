package orchestration

import (
	"context"
	"testing"
)

// fakeSuspendable is an AgentRunner that also implements
// SuspendableChildFlowRunner with scripted suspend/resume behaviour.
type fakeSuspendable struct {
	okRunner
	firstWaiting    bool // first RunChildFlowResumable returns waiting
	resumeOut       string
	resumeCalls     int
	lastResumeInput string
}

func (f *fakeSuspendable) RunChildFlowResumable(_ context.Context, _, input string) (out, childRunID string, waiting bool, err error) {
	if f.firstWaiting {
		return "", "childRun1", true, nil
	}
	return "child-done:" + input, "childRun1", false, nil
}

func (f *fakeSuspendable) ResumeChildFlow(_ context.Context, _, input string) (out string, waiting bool, err error) {
	f.resumeCalls++
	f.lastResumeInput = input
	return f.resumeOut, false, nil
}

func subflowGraph() Graph {
	return Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "sub"},
			{ID: "sub", Type: NodeSubflow, FlowRef: "FLWchild", Template: "{{input}}", Next: "end"},
			{ID: "end", Type: NodeEnd},
		},
	}
}

// TestSubflow_AwaitPropagation drives the two-phase flow: the child suspends at
// await-input, so the parent parks at the subflow node (WaitingAt + SubflowRun);
// feeding the parent then resumes the child, which completes and the parent
// advances.
func TestSubflow_AwaitPropagation(t *testing.T) {
	r := &fakeSuspendable{firstWaiting: true, resumeOut: "final-out"}
	eng := NewEngine(r)

	// Phase 1: child suspends → parent parks.
	st, err := eng.Run(context.Background(), subflowGraph(), "X", NewState(subflowGraph()), nil)
	if err != nil {
		t.Fatalf("phase 1 failed: %v", err)
	}
	if st.WaitingAt != "sub" || st.SubflowRun != "childRun1" || st.Current != "sub" {
		t.Fatalf("parent should be parked inside the subflow, got WaitingAt=%q SubflowRun=%q Current=%q", st.WaitingAt, st.SubflowRun, st.Current)
	}

	// Phase 2: feed the parent → the child resumes and completes → parent advances.
	st.Last = "user answer"
	final, err := eng.Run(context.Background(), subflowGraph(), "X", st, nil)
	if err != nil {
		t.Fatalf("phase 2 failed: %v", err)
	}
	if r.resumeCalls != 1 || r.lastResumeInput != "user answer" {
		t.Errorf("delivered input should reach the child, got calls=%d input=%q", r.resumeCalls, r.lastResumeInput)
	}
	if final.WaitingAt != "" || final.SubflowRun != "" {
		t.Errorf("parent should be unparked after the child completes, got WaitingAt=%q SubflowRun=%q", final.WaitingAt, final.SubflowRun)
	}
	if final.Last != "final-out" || final.Current != "" {
		t.Errorf("expected the child output then a finished run, got Last=%q Current=%q", final.Last, final.Current)
	}
}

// TestSubflow_NoSuspendRunsThrough verifies the resumable runner still advances
// straight through when the child does NOT suspend.
func TestSubflow_NoSuspendRunsThrough(t *testing.T) {
	r := &fakeSuspendable{firstWaiting: false}
	final, err := NewEngine(r).Run(context.Background(), subflowGraph(), "X", NewState(subflowGraph()), nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if final.WaitingAt != "" || final.Last != "child-done:X" || final.Current != "" {
		t.Errorf("non-suspending subflow should run straight through, got WaitingAt=%q Last=%q Current=%q", final.WaitingAt, final.Last, final.Current)
	}
}
