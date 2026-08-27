package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

// base builds a small graph: start → collect → fan (a,b) → synth → end.
func base(now time.Time) FlowRunInput {
	// The persisted model mixes units: FlowRun timestamps and TraceEntry.At are
	// unix SECONDS, StartMs/EndMs are MILLIseconds. The fixture mirrors that
	// exactly — an earlier all-millis fixture hid a real rendering bug.
	t0 := now.Add(-2 * time.Minute).Unix()
	t0ms := now.Add(-2 * time.Minute).UnixMilli()
	g := orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "collect"},
			{ID: "collect", Type: orchestration.NodeAgent, Title: "collect", AgentID: "A", Next: "fan"},
			{ID: "fan", Type: orchestration.NodeParallel, Title: "fan", Parallel: []string{"a", "b"}, JoinNext: "synth"},
			{ID: "a", Type: orchestration.NodeAgent, Title: "a", AgentID: "A"},
			{ID: "b", Type: orchestration.NodeAgent, Title: "b", AgentID: "A"},
			{ID: "synth", Type: orchestration.NodeAgent, Title: "synth", AgentID: "A", Next: "end"},
			{ID: "end", Type: orchestration.NodeEnd},
		},
	}
	st := orchestration.State{
		Current: "synth",
		Outputs: map[string]string{},
		Trace: []orchestration.TraceEntry{
			{NodeID: "start", Type: orchestration.NodeStart, Title: "start", At: t0},
			{NodeID: "collect", Type: orchestration.NodeAgent, Title: "collect", Output: "ok", At: t0 + 31},
			{NodeID: "a", Type: orchestration.NodeAgent, Title: "a", Output: "x", StartMs: t0ms + 31_200, EndMs: t0ms + 60_000, At: t0 + 60},
			{NodeID: "b", Type: orchestration.NodeAgent, Title: "b", Output: "y", StartMs: t0ms + 31_200, EndMs: t0ms + 79_200, At: t0 + 79},
			{NodeID: "fan", Type: orchestration.NodeParallel, Title: "fan", Output: "joined", At: t0 + 79},
		},
	}
	return FlowRunInput{
		Run: db.FlowRun{
			ID: "RUN7f2", FlowID: "FLW1", Status: db.FlowRunning,
			CreatedAt: t0, UpdatedAt: t0 + 79, State: "unused",
		},
		Flow:  db.Flow{ID: "FLW1", Name: "research-pipeline"},
		Graph: g,
		State: st,
		Now:   now,
	}
}

func TestFlowRunCardFoldsParallelAndShowsCurrent(t *testing.T) {
	now := time.Now()
	v, err := ProjectFlowRun(base(now), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()

	// 5 distinct nodes touched out of 7 in the graph.
	if !strings.Contains(v.Header, "5/7 node") {
		t.Errorf("header lacks progress: %q", v.Header)
	}
	if !strings.Contains(v.Header, "RUNNING") {
		t.Errorf("header lacks status: %q", v.Header)
	}
	// The fan-out folds into one segment carrying its wall time (79.2−31.2 = 48s),
	// and the children must NOT appear as their own segments.
	if !strings.Contains(txt, "parallel:fan[2/2✓ 48s]") {
		t.Errorf("parallel not folded:\n%s", txt)
	}
	if strings.Contains(txt, "agent:a✓") {
		t.Errorf("parallel child leaked into chain:\n%s", txt)
	}
	// The node the run is parked on has no trace entry yet but must still show.
	if !strings.Contains(txt, "agent:synth⚡RUNNING") {
		t.Errorf("current node missing:\n%s", txt)
	}
	// Sequential duration is derived from the gap between trace stamps.
	if !strings.Contains(txt, "agent:collect✓(31s)") {
		t.Errorf("sequential duration missing:\n%s", txt)
	}
	if v.Tokens == 0 {
		t.Error("token estimate not filled")
	}
}

func TestFlowRunTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectFlowRun(base(time.Now()), LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
	if strings.Contains(v.Text(), "\n") && !strings.Contains(v.Header, "child of") {
		t.Errorf("tiny level should be one line: %q", v.Text())
	}
}

func TestFlowRunFailureSurfacesErrorAndHandle(t *testing.T) {
	now := time.Now()
	in := base(now)
	in.Run.Status = db.FlowFailure
	in.Run.Error = "node \"synth\": provider 429 rate_limit"

	v, err := ProjectFlowRun(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "✗FAILED") {
		t.Errorf("failure marker missing:\n%s", txt)
	}
	if !strings.Contains(txt, "rate_limit") {
		t.Errorf("error text missing:\n%s", txt)
	}
	// A failed run must offer a way to open the node that failed.
	found := false
	for _, h := range v.Handles {
		if h.Ref.Sub == "fan" {
			found = true
		}
	}
	if !found {
		t.Errorf("no drill-down handle for the failing node: %+v", v.Handles)
	}
}

func TestFlowRunWaitingIsVisible(t *testing.T) {
	now := time.Now()
	in := base(now)
	in.Run.Status = db.FlowWaiting
	in.State.WaitingAt = "synth"

	v, err := ProjectFlowRun(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "⏸WAITING") || !strings.Contains(txt, "await-input node:synth") {
		t.Errorf("waiting state not surfaced:\n%s", txt)
	}
}

func TestFlowRunLongChainReportsElision(t *testing.T) {
	now := time.Now()
	in := base(now)
	in.Graph.Nodes = nil
	in.State.Trace = nil
	t0 := now.Add(-time.Minute).Unix()
	for i := 0; i < 60; i++ {
		id := "n" + string(rune('a'+i%26))
		in.Graph.Nodes = append(in.Graph.Nodes, orchestration.Node{ID: id, Type: orchestration.NodeTransform})
		in.State.Trace = append(in.State.Trace, orchestration.TraceEntry{
			NodeID: id, Type: orchestration.NodeTransform, Title: id, At: t0 + int64(i),
		})
	}
	in.Run.Status = db.FlowSuccess
	in.State.Current = ""

	v, err := ProjectFlowRun(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Elided == 0 {
		t.Fatal("over-long chain must report elision")
	}
	if !strings.Contains(v.Text(), "node gizlendi") {
		t.Errorf("elision not rendered:\n%s", v.Text())
	}
}

func TestFlowRunRejectsEmptyInput(t *testing.T) {
	if _, err := ProjectFlowRun(FlowRunInput{}, LevelCard); err == nil {
		t.Fatal("expected an error for an empty run, got a view")
	}
}
