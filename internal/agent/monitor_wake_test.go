package agent

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestWakeNow_ArmsThroughTheSameChain pins that an event wake (what a monitor
// match produces) goes through the ORDINARY wake machinery: it persists a one-shot
// schedule bound to the originating session, so the scheduler's
// fireWake -> ConsumeOneShotSchedule -> deliverWake path delivers it and its
// at-most-once guarantee applies. There must be no parallel delivery path.
func TestWakeNow_ArmsThroughTheSameChain(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	reloaded := 0
	rt.SetScheduleReloader(func(context.Context) error { reloaded++; return nil })
	ctx := context.Background()

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Gözcü", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	id, err := rt.WakeNow(ctx, sess.ID, agent.ID, "Monitor mon1 matched", "monitor mon1 matched on shell bg1")
	if err != nil {
		t.Fatalf("WakeNow: %v", err)
	}
	if id == "" {
		t.Fatal("WakeNow returned no schedule id")
	}
	if reloaded != 1 {
		t.Fatalf("scheduler re-armed %d times, want 1", reloaded)
	}

	all, err := rt.db.ListSchedules(ctx)
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("want exactly 1 one-shot row, got %d", len(all))
	}
	sc := all[0]
	if !sc.OneShot || sc.SessionID != sess.ID || sc.AgentID != agent.ID || sc.CronExpr != "" {
		t.Fatalf("event wake is not a session-bound one-shot: %+v", sc)
	}
	if sc.Prompt != "Monitor mon1 matched" {
		t.Fatalf("prompt not carried: %q", sc.Prompt)
	}
	// Unlike ScheduleWake, an event wake is NOT floored to MinWakeDelaySec — the
	// scheduler's own clamp (1s for an overdue row) decides. Anything further out
	// would defeat the point of reacting to an event.
	if sc.FireAt > time.Now().Add(time.Duration(MinWakeDelaySec)*time.Second).Unix() {
		t.Fatalf("event wake was delayed like a scheduled one: FireAt=%d", sc.FireAt)
	}

	// The row the scheduler would consume is claimable exactly once — the shared
	// at-most-once guarantee, not a monitor-specific one.
	if _, claimed, err := rt.db.ConsumeOneShotSchedule(ctx, sc.ID); err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}
	if _, claimed, err := rt.db.ConsumeOneShotSchedule(ctx, sc.ID); err != nil || claimed {
		t.Fatalf("second claim must fail: claimed=%v err=%v", claimed, err)
	}
}

// TestWakeNow_RejectsMissingContext keeps the preconditions loud: without a
// session there is nowhere to wake into, and without a prompt there is nothing to
// do on waking. Neither may be swallowed.
func TestWakeNow_RejectsMissingContext(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.SetScheduleReloader(func(context.Context) error { return nil })
	ctx := context.Background()
	if _, err := rt.WakeNow(ctx, "", "AG1", "x", ""); err == nil {
		t.Error("empty session id must be rejected")
	}
	if _, err := rt.WakeNow(ctx, "SES1", "AG1", "   ", ""); err == nil {
		t.Error("empty prompt must be rejected")
	}
}

// TestReleaseSessionRuntimeState_DropsPerSessionState is the leak guard: the
// per-session managers the Runtime caches across turns must be removed when the
// session is deleted, and the monitor manager must be CLOSED (its poll goroutine
// stopped) so it can never wake a session that no longer exists.
func TestReleaseSessionRuntimeState_DropsPerSessionState(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	const sessionID = "SES-release"

	// Populate all three maps through the production accessors.
	if got := rt.readTrackerFor(sessionID); got == nil {
		t.Fatal("readTrackerFor returned nil for a real session")
	}
	shellMgr := rt.shellMgrFor(sessionID)
	if shellMgr == nil {
		t.Fatal("shellMgrFor returned nil for a real session")
	}
	monMgr := rt.monitorMgrFor(sessionID, "AG1")
	if monMgr == nil {
		t.Fatal("monitorMgrFor returned nil for a real session")
	}
	// Arm a monitor so the manager owns a live source + poll goroutine.
	src := &countingSource{}
	if _, err := monMgr.Start(src, "never-matches", time.Second, 0); err != nil {
		t.Fatalf("arm monitor: %v", err)
	}

	for _, m := range []struct {
		name string
		sm   *sync.Map
	}{
		{"readTrackers", &rt.readTrackers},
		{"shellMgrs", &rt.shellMgrs},
		{"monitorMgrs", &rt.monitorMgrs},
	} {
		if _, ok := m.sm.Load(sessionID); !ok {
			t.Fatalf("%s has no entry before release", m.name)
		}
	}

	rt.ReleaseSessionRuntimeState(sessionID)

	for _, m := range []struct {
		name string
		sm   *sync.Map
	}{
		{"readTrackers", &rt.readTrackers},
		{"shellMgrs", &rt.shellMgrs},
		{"monitorMgrs", &rt.monitorMgrs},
	} {
		if _, ok := m.sm.Load(sessionID); ok {
			t.Errorf("%s still holds the deleted session — leak", m.name)
		}
	}
	// The monitor's source was closed, i.e. the manager really shut down rather than
	// just being dropped from the map.
	if src.closed() == 0 {
		t.Error("monitor source was not closed on release — the poll goroutine leaked")
	}
	// Releasing twice, or releasing a session that never had state, is a no-op.
	rt.ReleaseSessionRuntimeState(sessionID)
	rt.ReleaseSessionRuntimeState("SES-never-existed")
	rt.ReleaseSessionRuntimeState("")
}

// countingSource is an inert monitor source that only records that it was closed.
type countingSource struct {
	mu sync.Mutex
	n  int
}

func (c *countingSource) Poll(context.Context) ([]tools.MonitorEvent, bool, string, error) {
	return nil, false, "", nil
}

func (c *countingSource) Describe() string { return "counting" }

func (c *countingSource) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func (c *countingSource) closed() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}
