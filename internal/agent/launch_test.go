package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestLaunchRun_ValidatesTarget verifies the dispatch validates the target
// agent before spawning: an unknown agent fails on the session path without a
// live provider.
func TestLaunchRun_ValidatesTarget(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()

	res, err := rt.LaunchRun(ctx, RunSpec{Trigger: TriggerAutomationTag, AgentID: "AGT-nope", Input: "x", Autonomous: true})
	if err == nil || !strings.Contains(err.Error(), "target agent gone") {
		t.Fatalf("spec should fail on the session path, got res=%+v err=%v", res, err)
	}
	if res.Driver != "session" {
		t.Errorf("expected driver=session, got %q", res.Driver)
	}
}

// TestLaunchRun_PauseGate verifies the shared launchGate: while the workspace
// autonomy brake is engaged, an AUTONOMOUS launch is refused up front (before any
// session is created), while a MANUAL launch bypasses the brake and proceeds to
// normal target validation.
func TestLaunchRun_PauseGate(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	rt.SetPaused(true)

	res, err := rt.LaunchRun(ctx, RunSpec{Trigger: TriggerSchedule, AgentID: "AGT-nope", Input: "x", Autonomous: true})
	if !errors.Is(err, ErrAutonomyPaused) {
		t.Fatalf("paused autonomous launch should return ErrAutonomyPaused, got res=%+v err=%v", res, err)
	}
	if res.Driver != "session" {
		t.Errorf("expected driver=session on the refusal, got %q", res.Driver)
	}

	_, err = rt.LaunchRun(ctx, RunSpec{Trigger: TriggerManual, AgentID: "AGT-nope", Input: "x", Autonomous: false})
	if err == nil || !strings.Contains(err.Error(), "target agent gone") {
		t.Fatalf("manual launch should bypass the pause gate, got err=%v", err)
	}
}
