package agent

import (
	"context"
	"strings"
	"testing"
)

// TestLaunchRun_RoutesByDriver verifies the unified dispatch decision: a spec with
// a FlowID takes the flow branch (validated via GetFlow), and a spec without one
// takes the session branch (validated via GetAgent). Both validation errors prove
// the routing without needing a live provider.
func TestLaunchRun_RoutesByDriver(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()

	// Flow driver: an unknown flow id must fail on the flow path.
	res, err := rt.LaunchRun(ctx, RunSpec{Trigger: TriggerSchedule, FlowID: "FLW-nope", Input: "x", Autonomous: true})
	if err == nil || !strings.Contains(err.Error(), "target flow gone") {
		t.Fatalf("flow-driver spec should fail on the flow path, got res=%+v err=%v", res, err)
	}
	if res.Driver != "flow" {
		t.Errorf("expected driver=flow, got %q", res.Driver)
	}

	// Session driver: no FlowID + an unknown agent must fail on the session path.
	res, err = rt.LaunchRun(ctx, RunSpec{Trigger: TriggerAutomationTag, AgentID: "AGT-nope", Input: "x", Autonomous: true})
	if err == nil || !strings.Contains(err.Error(), "target agent gone") {
		t.Fatalf("session-driver spec should fail on the session path, got res=%+v err=%v", res, err)
	}
	if res.Driver != "session" {
		t.Errorf("expected driver=session, got %q", res.Driver)
	}
}
