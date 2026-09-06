package api

import (
	"context"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/agent"
)

func registerTrackedRun(t *testing.T, runs *chatRuns, wsID, sessionID string) (*agent.ActivityTracker, func()) {
	t.Helper()
	ctx, stop := agent.WithActivityTimeout(context.Background(), 0, time.Second)
	tracker := agent.ActivityTrackerFrom(ctx)
	run := runs.register(sessionID, sessionID, wsID, func() {})
	run.setActivityTracker(tracker, time.Second)
	return tracker, func() {
		runs.unregister(sessionID)
		stop()
	}
}

func TestTurnIdleForFallsBackToStart(t *testing.T) {
	s := &Server{runs: newChatRuns()}
	started := time.Now().Add(-90 * time.Second)
	idle, ok := s.turnIdleFor("WS1", "SES", started)
	if !ok || idle < 90*time.Second {
		t.Fatalf("idle = %v ok=%v", idle, ok)
	}
}

func TestTurnIdleForResetsOnSemanticProgress(t *testing.T) {
	runs := newChatRuns()
	s := &Server{runs: runs}
	tracker, cleanup := registerTrackedRun(t, runs, "WS1", "SES")
	defer cleanup()
	tracker.ObserveStep(agent.TurnStep{Kind: agent.StepDelta, Text: "new"})
	idle, ok := s.turnIdleFor("WS1", "SES", time.Now().Add(-time.Hour))
	if !ok || idle > time.Second {
		t.Fatalf("idle = %v ok=%v", idle, ok)
	}
}

func TestTurnIdleForIgnoresAnotherRunProgress(t *testing.T) {
	runs := newChatRuns()
	target, cleanupTarget := registerTrackedRun(t, runs, "WS1", "SES1")
	defer cleanupTarget()
	other, cleanupOther := registerTrackedRun(t, runs, "WS1", "SES2")
	defer cleanupOther()
	before := target.Snapshot()
	other.ObserveStep(agent.TurnStep{Kind: agent.StepDelta, Text: "other run"})
	time.Sleep(10 * time.Millisecond)
	after := target.Snapshot()
	if after.Sequence != before.Sequence || !after.LastProgressAt.Equal(before.LastProgressAt) {
		t.Fatalf("other run advanced target: before=%+v after=%+v", before, after)
	}
}

func TestInboxWatchdogDefaultsWithoutTunables(t *testing.T) {
	s := &Server{}
	if got, want := s.inboxTurnIdleWatchdog(), agent.DefaultTurnIdleWatchdogMinutes*time.Minute; got != want {
		t.Fatalf("idle window = %v, want %v", got, want)
	}
}
