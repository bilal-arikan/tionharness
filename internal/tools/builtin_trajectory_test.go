package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestTrajectoryToolDispatch: the tool refuses to run outside a coordinator
// turn, routes each action to its function, and refuses the declaring actions
// on a session whose runner has only Get (a sub-coordinator).
func TestTrajectoryToolDispatch(t *testing.T) {
	tool := NewTrajectoryTool()
	if _, err := tool.Call(context.Background(), json.RawMessage(`{"action":"get"}`)); err == nil || !strings.Contains(err.Error(), "coordinator") {
		t.Fatalf("outside a coordinator turn: %v", err)
	}
	var got []string
	full := &TrajectoryFuncs{
		Get: func(context.Context) (string, error) { got = append(got, "get"); return "graph", nil },
		Plan: func(_ context.Context, phases []TrajectoryPhaseInput) (string, error) {
			got = append(got, "plan:"+phases[0].ID+"/"+phases[0].Gate.Kind)
			return "planned", nil
		},
		Phase: func(_ context.Context, id, state, reason string) (string, error) {
			got = append(got, "phase:"+id+"/"+state+"/"+reason)
			return "moved", nil
		},
		Finish: func(_ context.Context, status, reason string) (string, error) {
			got = append(got, "finish:"+status)
			return "finished", nil
		},
	}
	ctx := WithCoordination(context.Background(), &CoordinationFuncs{Trajectory: full})
	calls := []string{
		`{"action":"get"}`,
		`{}`,
		`{"action":"plan","phases":[{"id":"plan","gate":{"kind":"artifact","value":"plan"}}]}`,
		`{"action":"phase","id":"plan","state":"Active","reason":"go"}`,
		`{"action":"finish"}`,
		`{"action":"finish","status":"failed"}`,
	}
	for _, c := range calls {
		if _, err := tool.Call(ctx, json.RawMessage(c)); err != nil {
			t.Fatalf("%s: %v", c, err)
		}
	}
	want := "get,get,plan:plan/artifact,phase:plan/active/go,finish:done,finish:failed"
	if strings.Join(got, ",") != want {
		t.Fatalf("dispatch = %v", got)
	}
	for _, bad := range []string{
		`{"action":"plan"}`,
		`{"action":"phase","id":"plan"}`,
		`{"action":"dance"}`,
	} {
		if _, err := tool.Call(ctx, json.RawMessage(bad)); err == nil {
			t.Fatalf("%s must error", bad)
		}
	}
	// Read-only runner (sub-coordinator).
	sub := WithCoordination(context.Background(), &CoordinationFuncs{Trajectory: &TrajectoryFuncs{Get: full.Get}})
	if _, err := tool.Call(sub, json.RawMessage(`{"action":"get"}`)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{`{"action":"plan","phases":[{"id":"x"}]}`, `{"action":"phase","id":"x","state":"done"}`, `{"action":"finish"}`} {
		if _, err := tool.Call(sub, json.RawMessage(c)); err == nil || !strings.Contains(err.Error(), "ROOT") {
			t.Fatalf("%s on a sub-coordinator: %v", c, err)
		}
	}
	if !IsCoordinationTool(TrajectoryToolName) {
		t.Fatal("trajectory must be part of the coordination surface (session-gated, not allowlist-gated)")
	}
}
