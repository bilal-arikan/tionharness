package agent

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
)

// TestFlowRecordsDistinctSessionPerRun verifies the per-run session model: each
// recorded flow run lands in its OWN session (keyed for attribution by
// SourceID = flow.ID but never reused), so a run's transcript — and its
// "Akış olarak gör" reification — shows exactly one run.
func TestFlowRecordsDistinctSessionPerRun(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a := newFlowAgent(t, rt, "worker")
	g := orchestration.Graph{
		Start: "n1",
		Nodes: []orchestration.Node{{ID: "n1", Type: orchestration.NodeAgent, AgentID: a.ID, Prompt: "{{input}}"}},
	}
	flowID := createFlow(t, rt, g)
	flow, err := rt.db.GetFlow(ctx, flowID)
	if err != nil {
		t.Fatalf("get flow: %v", err)
	}

	// Two runs → two turn recordings with no pre-created session id (the fallback
	// create path). Each must produce a distinct flow session.
	run1 := db.FlowRun{ID: "R1", FlowID: flowID, Status: db.FlowSuccess, Output: "out1"}
	run2 := db.FlowRun{ID: "R2", FlowID: flowID, Status: db.FlowSuccess, Output: "out2"}
	s1 := rt.recordFlowSessionTurn(ctx, flow, run1, "first", nil, "", false)
	s2 := rt.recordFlowSessionTurn(ctx, flow, run2, "second", nil, "", false)

	if s1 == "" || s2 == "" {
		t.Fatalf("expected both runs to record a session, got %q / %q", s1, s2)
	}
	if s1 == s2 {
		t.Fatalf("per-run sessions must be distinct, both were %q (reused, not per-run)", s1)
	}

	// Both sessions are flow-kind and attribute back to the flow via SourceID.
	for _, id := range []string{s1, s2} {
		sess, err := rt.db.GetSession(ctx, id)
		if err != nil {
			t.Fatalf("get session %s: %v", id, err)
		}
		if sess.Kind != "flow" {
			t.Errorf("session %s kind = %q, want flow", id, sess.Kind)
		}
		if sess.SourceID != flowID {
			t.Errorf("session %s sourceID = %q, want flow id %q (graph/executions attribution)", id, sess.SourceID, flowID)
		}
		if sess.MessageCount != 2 {
			t.Errorf("session %s should hold exactly one run (user+assistant = 2 msgs), got %d", id, sess.MessageCount)
		}
	}
}
