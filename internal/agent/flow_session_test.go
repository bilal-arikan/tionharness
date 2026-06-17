package agent

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/orchestration"
)

// TestFlowStateToSteps verifies a finished flow run's persisted state maps to a
// per-node turn trace, so a flow run renders like a normal chat turn's activity.
func TestFlowStateToSteps(t *testing.T) {
	st := orchestration.State{
		Trace: []orchestration.TraceEntry{
			{NodeID: "n1", Title: "Plan", Output: "step one"},
			{NodeID: "n2", Title: "", Output: "step two"}, // empty title → node id
		},
	}
	data, _ := json.Marshal(st)
	fr := db.FlowRun{State: string(data), Status: db.FlowSuccess}

	steps := flowStateToSteps(fr, nil)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Kind != StepText || steps[0].Text == "" {
		t.Fatalf("expected non-empty text step, got %+v", steps[0])
	}
	// Falls back to node id when the title is empty.
	if want := "**n2**"; steps[1].Text[:len(want)] != want {
		t.Fatalf("expected node-id title fallback, got %q", steps[1].Text)
	}
}

// TestFlowStateToStepsSetupError surfaces a setup failure as a single error step.
func TestFlowStateToStepsSetupError(t *testing.T) {
	steps := flowStateToSteps(db.FlowRun{}, errors.New("bad graph"))
	if len(steps) != 1 || steps[0].Kind != StepError {
		t.Fatalf("expected one error step, got %+v", steps)
	}
}

// TestFlowStateToStepsFailureAppendsError adds an error step when the run failed.
func TestFlowStateToStepsFailureAppendsError(t *testing.T) {
	st := orchestration.State{Trace: []orchestration.TraceEntry{{NodeID: "n1", Output: "x"}}}
	data, _ := json.Marshal(st)
	fr := db.FlowRun{State: string(data), Status: db.FlowFailure, Error: "node blew up"}

	steps := flowStateToSteps(fr, nil)
	if len(steps) != 2 || steps[1].Kind != StepError {
		t.Fatalf("expected trailing error step, got %+v", steps)
	}
}
