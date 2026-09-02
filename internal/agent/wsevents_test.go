package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

// collectWS attaches a bus to the runtime and returns a drain that collects the
// ws:* events published so far (waiting briefly for in-flight publishes).
func collectWS(t *testing.T, rt *Runtime) func() []events.Event {
	t.Helper()
	bus := events.NewBus()
	rt.bus = bus
	_, ch := bus.Subscribe()
	var got []events.Event
	return func() []events.Event {
		deadline := time.After(300 * time.Millisecond)
		for {
			select {
			case e := <-ch:
				if events.IsWorkspaceStream(e.Type) {
					got = append(got, e)
				}
			case <-deadline:
				return got
			}
		}
	}
}

func decodeData[T any](t *testing.T, e events.Event) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(e.Data, &v); err != nil {
		t.Fatalf("decode %s data: %v (%s)", e.Type, err, e.Data)
	}
	return v
}

// TestSessionHookEmitsLifecycleEvents: wiring the store hooks to the runtime
// turns session create/state/delete into ws:session_lifecycle events stamped
// with the workspace and carrying the session's origin.
func TestSessionHookEmitsLifecycleEvents(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.wsID = "WS-test"
	drain := collectWS(t, rt)
	rt.db.SetSessionHook(rt.OnSessionChange)
	rt.db.SetTrajectoryHook(rt.OnTrajectoryChange)
	ctx := context.Background()

	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic", Model: "m"})
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Title: "T", CoordinatorMode: true})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := rt.db.SetSessionState(ctx, sess.ID, "archived"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	tr, err := rt.db.CreateTrajectory(ctx, db.Trajectory{RootSessionID: sess.ID, TemplateRef: "r@1"})
	if err != nil {
		t.Fatalf("create trajectory: %v", err)
	}
	if err := rt.db.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got := drain()
	var lifecycle []SessionLifecyclePayload
	var traj []TrajectoryPayload
	for _, e := range got {
		if e.WorkspaceID != "WS-test" {
			t.Fatalf("event %s not stamped with the workspace: %q", e.Type, e.WorkspaceID)
		}
		switch e.Type {
		case events.TypeWSSessionLifecycle:
			p := decodeData[SessionLifecyclePayload](t, e)
			if p.SessionID == sess.ID {
				lifecycle = append(lifecycle, p)
			}
		case events.TypeWSTrajectory:
			traj = append(traj, decodeData[TrajectoryPayload](t, e))
		}
	}
	wantOps := []string{db.SessionOpCreate, db.SessionOpState, db.SessionOpDelete}
	if len(lifecycle) != len(wantOps) {
		t.Fatalf("lifecycle events = %+v, want ops %v", lifecycle, wantOps)
	}
	for i, op := range wantOps {
		if lifecycle[i].Op != op {
			t.Fatalf("lifecycle[%d].Op = %q, want %q", i, lifecycle[i].Op, op)
		}
	}
	if lifecycle[0].Origin.Kind != db.OriginUser || !lifecycle[0].Coordinator || lifecycle[0].RootSessionID != sess.ID {
		t.Fatalf("create payload = %+v, want user origin, coordinator, self root", lifecycle[0])
	}
	if lifecycle[1].PrevState != "active" || lifecycle[1].State != "archived" {
		t.Fatalf("state payload = %+v, want active→archived", lifecycle[1])
	}
	// Trajectory: create, then the root delete drops it.
	if len(traj) != 2 || traj[0].Op != db.TrajectoryOpCreate || traj[0].TrajectoryID != tr.ID || traj[0].Revision != 1 || traj[1].Op != db.TrajectoryOpDelete {
		t.Fatalf("trajectory events = %+v, want create(rev 1) then delete", traj)
	}
}

// TestFlowRunEmitsStatusEvents: a recorded flow run announces running at
// creation (already linked to its session) and its terminal status at the end.
func TestFlowRunEmitsStatusEvents(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.wsID = "WS-test"
	drain := collectWS(t, rt)
	configureSessionCtxProvider(rt)
	ctx := context.Background()

	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "flow node", Provider: "flow-session-ctx-test", Model: "test"})
	g := orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "n1"},
			{ID: "n1", Type: orchestration.NodeAgent, AgentID: a.ID, Prompt: "{{input}}"},
		},
	}
	flowID := createFlow(t, rt, g)
	run, sessionID, err := rt.RunFlowRecorded(ctx, flowID, "go", false, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var statuses []FlowRunPayload
	for _, e := range drain() {
		if e.Type == events.TypeWSFlowRun {
			statuses = append(statuses, decodeData[FlowRunPayload](t, e))
		}
	}
	if len(statuses) != 2 {
		t.Fatalf("flow_run events = %+v, want running then terminal", statuses)
	}
	if statuses[0].Status != db.FlowRunning || statuses[0].RunID != run.ID || statuses[0].SessionID != sessionID || statuses[0].RootRunID != run.ID {
		t.Fatalf("start event = %+v, want running/%s linked to %s", statuses[0], run.ID, sessionID)
	}
	if statuses[1].Status != db.FlowSuccess || statuses[1].RunID != run.ID {
		t.Fatalf("finish event = %+v, want success/%s", statuses[1], run.ID)
	}
	drainSpawns(t, rt)
}

// TestScheduleArmedEmitsFireAt: rebuilding the scheduler announces the next
// fire time of every enabled cron schedule.
func TestScheduleArmedEmitsFireAt(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.wsID = "WS-test"
	drain := collectWS(t, rt)
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic", Model: "m"})
	sc, err := rt.db.CreateSchedule(ctx, db.Schedule{Name: "nightly", AgentID: a.ID, CronExpr: "0 3 * * *", Prompt: "hi", Enabled: true})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	sched := NewScheduler(rt.db, rt, rt.logger)
	if err := sched.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	t.Cleanup(func() { sched.Stop() })
	var armed []ScheduleArmedPayload
	for _, e := range drain() {
		if e.Type == events.TypeWSScheduleArmed {
			armed = append(armed, decodeData[ScheduleArmedPayload](t, e))
		}
	}
	if len(armed) != 1 || armed[0].ScheduleID != sc.ID || armed[0].FireAt <= time.Now().Unix() {
		t.Fatalf("schedule_armed events = %+v, want one future fire for %s", armed, sc.ID)
	}
}
