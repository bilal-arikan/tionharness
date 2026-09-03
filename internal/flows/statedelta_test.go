package flows

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

func TestFlowStateDeltaAppendAndReplacement(t *testing.T) {
	base := orchestration.State{Current: "a", Outputs: map[string]string{"old": "x"}, Trace: []orchestration.TraceEntry{{NodeID: "old"}}, Spawned: map[string][]string{"spawn": {"RUN1"}}}
	w := NewStateDeltaWriter("checkpoint", 0, base)
	next := CloneState(base)
	next.Current = "b"
	next.Outputs["new"] = "large"
	delete(next.Outputs, "old")
	next.Trace = append(next.Trace, orchestration.TraceEntry{NodeID: "new"})
	next.Thread = append(next.Thread, orchestration.Msg{Role: "user", Text: "hello"})
	next.Spawned = nil
	delta, err := w.Next(next)
	if err != nil {
		t.Fatal(err)
	}
	if delta.Sequence != 1 || string(delta.Scalars["current"]) != `"b"` || delta.OutputsUpsert["new"] != "large" || len(delta.OutputsDelete) != 1 || len(delta.TraceAppend) != 1 || len(delta.ThreadAppend) != 1 || string(delta.Spawned) != "null" {
		t.Fatalf("unexpected delta: %#v", delta)
	}
}

func TestFlowStateDeltaRejectsShrinkingPrefixes(t *testing.T) {
	base := orchestration.State{Outputs: map[string]string{}, Trace: []orchestration.TraceEntry{{NodeID: "a"}}, Thread: []orchestration.Msg{{Role: "user", Text: "a"}}}
	for name, mutate := range map[string]func(*orchestration.State){
		"trace":  func(s *orchestration.State) { s.Trace = nil },
		"thread": func(s *orchestration.State) { s.Thread = nil },
	} {
		t.Run(name, func(t *testing.T) {
			w := NewStateDeltaWriter("checkpoint", 0, base)
			next := CloneState(base)
			mutate(&next)
			if _, err := w.Next(next); err == nil {
				t.Fatal("expected prefix error")
			}
		})
	}
}
