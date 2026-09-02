package agent

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

// TestRunFlowStampsSessionAtStart: the run row is linked to its transcript
// session — FlowRun.SessionID and Session.Origin.RunID — BEFORE the first node
// runs, not after the run finishes. Observed from the first node's "start" event.
func TestRunFlowStampsSessionAtStart(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	configureSessionCtxProvider(rt)
	ctx := context.Background()

	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "flow node", Provider: "flow-session-ctx-test", Model: "test"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	g := orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "n1"},
			{ID: "n1", Type: orchestration.NodeAgent, AgentID: a.ID, Prompt: "{{input}}"},
		},
	}
	flowID := createFlow(t, rt, g)

	var (
		mu         sync.Mutex
		atStart    *db.FlowRun
		atStartSes *db.Session
	)
	obs := orchestration.Observer(func(ev orchestration.NodeEvent) {
		if ev.Phase != "start" || ev.NodeID != "n1" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if atStart != nil {
			return
		}
		runs, _ := rt.db.ListFlowRuns(ctx, flowID)
		if len(runs) != 1 {
			t.Errorf("expected exactly one run at first node start, got %d", len(runs))
			return
		}
		run := runs[0]
		atStart = &run
		if run.SessionID != "" {
			if sess, err := rt.db.GetSession(ctx, run.SessionID); err == nil {
				atStartSes = &sess
			}
		}
	})

	run, sessionID, err := rt.RunFlowRecorded(ctx, flowID, "go", false, obs)
	if err != nil {
		t.Fatalf("run flow recorded: %v", err)
	}
	if run.Status != db.FlowSuccess {
		t.Fatalf("flow run status = %q (%s), want success", run.Status, run.Error)
	}
	mu.Lock()
	defer mu.Unlock()
	if atStart == nil {
		t.Fatal("the observer never saw the agent node start")
	}
	if atStart.SessionID != sessionID {
		t.Fatalf("FlowRun.SessionID at first node start = %q, want the transcript session %q (must be linked at creation, not at finish)", atStart.SessionID, sessionID)
	}
	if atStartSes == nil {
		t.Fatal("transcript session not readable at first node start")
	}
	o := atStartSes.Lineage()
	if o.Kind != db.OriginFlow || o.EntityID != flowID || o.RunID != run.ID {
		t.Fatalf("session origin at first node start = %+v, want flow/%s/%s", o, flowID, run.ID)
	}
	drainSpawns(t, rt)
}

// TestLaunchRunFlowCarriesLauncherOrigin: a flow launched by an automation keeps
// the AUTOMATION as its origin (entity + tripping session), with the run id
// filled in — the transcript session is not mislabelled as a plain flow start.
func TestLaunchRunFlowCarriesLauncherOrigin(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	configureSessionCtxProvider(rt)
	ctx := context.Background()

	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "flow node", Provider: "flow-session-ctx-test", Model: "test"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	trigger, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Title: "tagged"})
	if err != nil {
		t.Fatalf("create trigger session: %v", err)
	}
	g := orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "n1"},
			{ID: "n1", Type: orchestration.NodeAgent, AgentID: a.ID, Prompt: "{{input}}"},
		},
	}
	flowID := createFlow(t, rt, g)

	res, err := rt.LaunchRun(ctx, RunSpec{
		Trigger: TriggerAutomationTag, Input: "go", Autonomous: true, FlowID: flowID,
		Origin: &db.SessionOrigin{Kind: db.OriginAutomation, EntityID: "AUT7", TriggerSessionID: trigger.ID},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	sess, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	o := sess.Lineage()
	if o.Kind != db.OriginAutomation || o.EntityID != "AUT7" || o.TriggerSessionID != trigger.ID {
		t.Fatalf("origin = %+v, want automation/AUT7 tripped by %s", o, trigger.ID)
	}
	if o.RunID != res.FlowRun.ID || res.FlowRun.SessionID != sess.ID {
		t.Fatalf("run link: origin.RunID=%q run.SessionID=%q, want run %s ↔ session %s", o.RunID, res.FlowRun.SessionID, res.FlowRun.ID, sess.ID)
	}
	drainSpawns(t, rt)
}

// TestSpawnWorkerOriginIsCoordinator: a worker's origin names the coordinator
// that spawned it and the tree root, without the spawner passing an origin.
func TestSpawnWorkerOriginIsCoordinator(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	seedSystemAgents(t, rt)
	base, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Coord", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create base agent: %v", err)
	}
	coord := newTestCoordinator(t, rt, 0)
	defer waitWorkersSettled(t, rt, coord)

	r1, err := rt.SpawnWorker(ctx, coord, "explore", "map the code", base.ID, WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	worker, err := rt.db.GetSession(ctx, r1.SessionID)
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	o := worker.Lineage()
	if o.Kind != db.OriginCoordinator || o.TriggerSessionID != coord {
		t.Fatalf("worker origin = %+v, want coordinator←%s", o, coord)
	}
	if worker.RootSession() != coord {
		t.Fatalf("worker RootSession() = %q, want the root coordinator %q", worker.RootSession(), coord)
	}
	if o.At != worker.CreatedAt {
		t.Fatalf("origin.At = %d, want CreatedAt %d", o.At, worker.CreatedAt)
	}
}

// TestHandoffContinuationOrigin: a context-reset continuation is a handoff from
// the old session and stays in the old session's tree.
func TestHandoffContinuationOrigin(t *testing.T) {
	parent := db.Session{ID: "SES10", Kind: "chat", Title: "work",
		Origin: &db.SessionOrigin{Kind: db.OriginCoordinator, TriggerSessionID: "SES1", RootSessionID: "SES1"}}
	opts := handoffContinuationSpawnOpts(parent, HandoffOptions{})
	if opts.Origin == nil || opts.Origin.Kind != db.OriginHandoff {
		t.Fatalf("continuation origin = %+v, want handoff", opts.Origin)
	}
	if opts.Origin.TriggerSessionID != "SES10" {
		t.Fatalf("continuation trigger = %q, want the handed-off session SES10", opts.Origin.TriggerSessionID)
	}
	if opts.Origin.RootSessionID != "SES1" {
		t.Fatalf("continuation root = %q, want the parent's root SES1", opts.Origin.RootSessionID)
	}
	if opts.ParentSessionID != "SES10" {
		t.Fatalf("ParentSessionID must still mirror the handoff lineage, got %q", opts.ParentSessionID)
	}
}
