package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

const testRecipeSkill = `---
name: "Plan → Dev → Test"
kind: coordinator-workflow
pattern: custom
version: 3
phases:
  - id: plan
    profile: planner
    gate: { kind: artifact, value: plan }
  - id: code
    profile: coder
  - id: review
    profile: validator
    optional: true
watchers: [update-docs]
optimizer: recipe-optimizer
---
# body
`

// newTrajectoryRuntime builds a test runtime with the session hook wired the
// way workspace boot wires it (the binder listens there) and a structured
// coordinator recipe installed in the workspace skills tier.
func newTrajectoryRuntime(t *testing.T) (*Runtime, db.Agent) {
	t.Helper()
	workDir := filepath.Join(t.TempDir(), "workspace")
	skillDir := filepath.Join(workspaceSkillsDir(workDir), "plan-dev-test")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testRecipeSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, _ := newTestRuntime(t, workDir)
	rt.db.SetSessionHook(rt.OnSessionChange)
	rt.db.SetTrajectoryHook(rt.OnTrajectoryChange)
	seedSystemAgents(t, rt)
	base, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Coord", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create base agent: %v", err)
	}
	return rt, base
}

func waitTrajectory(t *testing.T, rt *Runtime, root string, ok func(db.Trajectory) bool) db.Trajectory {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		tr, err := rt.db.GetTrajectoryByRoot(context.Background(), root)
		if err == nil && ok(tr) {
			return tr
		}
		if time.Now().After(deadline) {
			t.Fatalf("trajectory of %s never reached the expected shape (err=%v): %+v", root, err, tr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestTrajectorySeededFromRecipeAtCreate: a root coordinator created with a
// recipe gets its trajectory at creation — declared phases, versioned ref,
// planned status — before anything runs.
func TestTrajectorySeededFromRecipeAtCreate(t *testing.T) {
	rt, base := newTrajectoryRuntime(t)
	ctx := context.Background()
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "chat", CoordinatorMode: true, CoordinatorWorkflow: "plan-dev-test@3"})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := rt.db.GetTrajectoryByRoot(ctx, sess.ID)
	if err != nil {
		t.Fatalf("trajectory not seeded at create: %v", err)
	}
	if tr.TemplateRef != "plan-dev-test@3" || tr.Status != db.TrajStatusPlanned || len(trajPhases(&tr)) != 3 {
		t.Fatalf("seeded trajectory = %+v", tr)
	}
	if n := trajNodePtr(&tr, "s:"+sess.ID); n == nil || n.Lane != 0 {
		t.Fatalf("root node = %+v", n)
	}
	// A worker session is never a root: no trajectory of its own.
	worker, _ := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "chat", CoordinatorSessionID: sess.ID, RootCoordinatorSessionID: sess.ID, Role: db.SessionRoleWorker, CoordinatorMode: true, CoordinatorWorkflow: "plan-dev-test@3"})
	if _, err := rt.db.GetTrajectoryByRoot(ctx, worker.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("worker must not own a trajectory, got err=%v", err)
	}
}

// TestTrajectoryBindsSpawnAndReport: spawning a worker under a recipe-driven
// coordinator auto-starts the first phase and binds the worker under it; the
// worker's report closes its node with a reported edge; archiving the root
// mid-plan abandons the trajectory.
func TestTrajectoryBindsSpawnAndReport(t *testing.T) {
	rt, base := newTrajectoryRuntime(t)
	ctx := context.Background()
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "chat", CoordinatorMode: true, CoordinatorWorkflow: "plan-dev-test"})
	if err != nil {
		t.Fatal(err)
	}
	coord := sess.ID
	res, err := rt.SpawnWorker(ctx, coord, "explore", "map the code", base.ID, WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	waitWorkersSettled(t, rt, coord)
	tr := waitTrajectory(t, rt, coord, func(tr db.Trajectory) bool {
		n := trajNodePtr(&tr, "s:"+res.SessionID)
		return n != nil && n.State != db.TrajStateActive
	})
	drainSpawns(t, rt)

	w := trajNodePtr(&tr, "s:"+res.SessionID)
	if w.Kind != db.TrajNodeSession || w.Origin != db.TrajOriginObserved || w.RefID != res.SessionID || w.Lane != 1 || w.PhaseID != "p:plan" || w.Label != res.AgentName {
		t.Fatalf("worker node = %+v", w)
	}
	// The test runtime has no real provider: the worker reports whatever status
	// it ends with (failed here); the binder records the reported status either way.
	if (w.State != db.TrajStateDone && w.State != db.TrajStateFailed) || w.EndMs == 0 {
		t.Fatalf("worker node after report = %+v", w)
	}
	if w.State == db.TrajStateFailed && w.Reason == "" {
		t.Fatalf("a failed worker must carry the reported status as reason: %+v", w)
	}
	if trajActivePhase(&tr) != "p:plan" || tr.Status != db.TrajStatusRunning {
		t.Fatalf("phase/status after first spawn = %s / %s", trajActivePhase(&tr), tr.Status)
	}
	var spawned, reported bool
	for _, e := range tr.Edges {
		switch {
		case e.From == "s:"+coord && e.To == "s:"+res.SessionID && e.Kind == db.TrajEdgeSpawned:
			spawned = true
		case e.From == "s:"+res.SessionID && e.To == "s:"+coord && e.Kind == db.TrajEdgeReported:
			reported = true
		}
	}
	if !spawned || !reported {
		t.Fatalf("edges = %+v, want spawned + reported", tr.Edges)
	}
	if tr.TemplateRef != "plan-dev-test@3" {
		t.Fatalf("bare slug must resolve to the versioned ref, got %q", tr.TemplateRef)
	}

	// Situation block pushes the trajectory to the coordinator.
	block := rt.coordinatorTrajectoryBlock(ctx, coord)
	for _, want := range []string{"<trajectory>", tr.ID, "plan ●", "Active phase plan: 0 worker(s) running", "trajectory{action:\"phase\""} {
		if !strings.Contains(block, want) {
			t.Fatalf("situation block missing %q:\n%s", want, block)
		}
	}
	if !strings.Contains(rt.coordinatorSituationBlock(ctx, coord), "<trajectory>") {
		t.Fatal("trajectory block must be part of the coordinator situation block")
	}

	// Archiving the root mid-plan abandons the trajectory.
	if err := rt.db.SetSessionState(ctx, coord, "archived"); err != nil {
		t.Fatal(err)
	}
	tr = waitTrajectory(t, rt, coord, func(tr db.Trajectory) bool { return tr.IsTerminal() })
	if tr.Status != db.TrajStatusAbandoned {
		t.Fatalf("status after archiving the root = %s, want abandoned", tr.Status)
	}
	if n := trajNodePtr(&tr, "s:"+coord); n.State != db.TrajStateDone || n.EndMs == 0 {
		t.Fatalf("root node after archive = %+v", n)
	}
}

// TestTrajectoryAgentPlannedThroughTool: a coordinator without a recipe gets
// its trajectory on the first spawn (root + worker only); the trajectory tool
// then declares a plan, moves phases and finishes it — and is registered in the
// native registry only for a coordinator session.
func TestTrajectoryAgentPlannedThroughTool(t *testing.T) {
	rt, base := newTrajectoryRuntime(t)
	ctx := context.Background()
	coord := newTestCoordinator(t, rt, 0)
	if _, err := rt.db.GetTrajectoryByRoot(ctx, coord); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("no recipe → nothing to seed at create, got err=%v", err)
	}
	res, err := rt.SpawnWorker(ctx, coord, "explore", "look around", base.ID, WorkerSpec{})
	if err != nil {
		t.Fatal(err)
	}
	waitWorkersSettled(t, rt, coord)
	waitTrajectory(t, rt, coord, func(tr db.Trajectory) bool {
		n := trajNodePtr(&tr, "s:"+res.SessionID)
		return n != nil && n.State != db.TrajStateActive
	})
	drainSpawns(t, rt)

	sess, _ := rt.db.GetSession(ctx, coord)
	tctx := rt.withCoordination(WithSessionID(ctx, coord), base)
	reg := rt.buildRegistry(tctx, base)
	if !reg.Has("trajectory") {
		t.Fatal("trajectory tool must be registered for a coordinator session")
	}
	call := func(args map[string]any) providers.ToolResult {
		input, _ := json.Marshal(args)
		return reg.Call(tctx, providers.ToolCall{ID: "c1", Name: "trajectory", Input: input})
	}
	if out := call(map[string]any{"action": "get"}); out.IsError || !strings.Contains(out.Content, "none declared") || !strings.Contains(out.Content, res.SessionID) {
		t.Fatalf("get = %+v", out)
	}
	out := call(map[string]any{"action": "plan", "phases": []map[string]any{
		{"id": "research", "profile": "explore"},
		{"id": "build", "profile": "coder", "gate": map[string]any{"kind": "verdict", "value": "PASS"}},
	}})
	if out.IsError || !strings.Contains(out.Content, "research ○ (explore) → build ○") {
		t.Fatalf("plan = %+v", out)
	}
	if out := call(map[string]any{"action": "phase", "id": "research", "state": "active"}); out.IsError || !strings.Contains(out.Content, "research ●") {
		t.Fatalf("phase active = %+v", out)
	}
	tr, _ := rt.db.GetTrajectoryByRoot(ctx, coord)
	if tr.Status != db.TrajStatusRunning || trajActivePhase(&tr) != "p:research" {
		t.Fatalf("after phase active: %s / %s", tr.Status, trajActivePhase(&tr))
	}
	if out := call(map[string]any{"action": "phase", "id": "ghost", "state": "done"}); !out.IsError {
		t.Fatalf("unknown phase must error, got %+v", out)
	}
	if out := call(map[string]any{"action": "finish", "status": "done"}); out.IsError || !strings.Contains(out.Content, "finished as done") {
		t.Fatalf("finish = %+v", out)
	}
	tr, _ = rt.db.GetTrajectoryByRoot(ctx, coord)
	if tr.Status != db.TrajStatusDone || trajNodePtr(&tr, "p:research").State != db.TrajStateDone || trajNodePtr(&tr, "s:"+coord).State != db.TrajStateDone {
		t.Fatalf("after finish: %+v", tr)
	}
	// A worker that is not a coordinator has no trajectory runner → no tool.
	wsess, _ := rt.db.GetSession(ctx, res.SessionID)
	if f := rt.trajectoryFuncsFor(wsess); f != nil {
		t.Fatalf("plain worker must not get trajectory funcs, got %+v", f)
	}
	// A sub-coordinator reads but never declares.
	sub := db.Session{ID: "SESSUB", CoordinatorMode: true, CoordinatorSessionID: coord, RootCoordinatorSessionID: coord, Origin: &db.SessionOrigin{Kind: db.OriginCoordinator, TriggerSessionID: coord, RootSessionID: coord}}
	if f := rt.trajectoryFuncsFor(sub); f == nil || f.Get == nil || f.Plan != nil || f.Phase != nil || f.Finish != nil {
		t.Fatalf("sub-coordinator funcs = %+v", f)
	}
	_ = sess
}

// TestTrajectoryAskGate: a durable ask parked on a tree member opens a human
// gate (trajectory waiting); releasing it closes the gate and resumes.
func TestTrajectoryAskGate(t *testing.T) {
	rt, base := newTrajectoryRuntime(t)
	ctx := context.Background()
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "chat", CoordinatorMode: true, CoordinatorWorkflow: "plan-dev-test"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := rt.db.CreateSessionAsk(ctx, db.SessionAsk{SessionID: sess.ID, AgentID: base.ID, Kind: "ask", CallID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	rt.bindAskToTrajectory(ask)
	tr, _ := rt.db.GetTrajectoryByRoot(ctx, sess.ID)
	g := trajNodePtr(&tr, "g:"+ask.ID)
	if g == nil || g.State != db.TrajStateActive || g.Gate == nil || g.Gate.Kind != "human" || tr.Status != db.TrajStatusWaiting {
		t.Fatalf("gate after ask = %+v / status %s", g, tr.Status)
	}
	var blocked bool
	for _, e := range tr.Edges {
		if e.From == "s:"+sess.ID && e.To == "g:"+ask.ID && e.Kind == db.TrajEdgeBlockedBy {
			blocked = true
		}
	}
	if !blocked {
		t.Fatalf("edges = %+v, want blocked_by", tr.Edges)
	}
	rt.ReleaseAsk(ask, db.SessionAskTimeout)
	tr, _ = rt.db.GetTrajectoryByRoot(ctx, sess.ID)
	g = trajNodePtr(&tr, "g:"+ask.ID)
	if g.State != db.TrajStateSkipped || g.Reason != db.SessionAskTimeout || tr.Status == db.TrajStatusWaiting {
		t.Fatalf("gate after release = %+v / status %s", g, tr.Status)
	}
	// Releasing twice is a no-op.
	rev := tr.Revision
	rt.ReleaseAsk(ask, db.SessionAskResolved)
	tr, _ = rt.db.GetTrajectoryByRoot(ctx, sess.ID)
	if g := trajNodePtr(&tr, "g:"+ask.ID); g.State != db.TrajStateSkipped {
		t.Fatalf("second release changed the gate: %+v (rev %d → %d)", g, rev, tr.Revision)
	}
}

// TestTrajectoryFlowRunBinding: a flow run whose transcript was triggered from
// a tree member becomes a flowrun node under the trigger, following the run's
// status; a run with no trigger in any trajectory is ignored.
func TestTrajectoryFlowRunBinding(t *testing.T) {
	rt, base := newTrajectoryRuntime(t)
	ctx := context.Background()
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "chat", CoordinatorMode: true, CoordinatorWorkflow: "plan-dev-test"})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := rt.db.CreateFlow(ctx, db.Flow{Name: "F"})
	if err != nil {
		t.Fatal(err)
	}
	transcript, _ := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "flow", SourceID: flow.ID, Title: "F run",
		Origin: &db.SessionOrigin{Kind: db.OriginFlow, EntityID: flow.ID, TriggerSessionID: sess.ID}})
	run, err := rt.db.CreateFlowRun(ctx, db.FlowRun{FlowID: flow.ID, SessionID: transcript.ID})
	if err != nil {
		t.Fatal(err)
	}
	rt.bindFlowRunToTrajectory(run)
	tr, _ := rt.db.GetTrajectoryByRoot(ctx, sess.ID)
	n := trajNodePtr(&tr, "r:"+run.ID)
	if n == nil || n.Kind != db.TrajNodeFlowRun || n.State != db.TrajStateActive || n.RefID != run.ID || n.Lane == 0 {
		t.Fatalf("flowrun node = %+v", n)
	}
	run.Status, run.Error = db.FlowFailure, "boom"
	rt.bindFlowRunToTrajectory(run)
	tr, _ = rt.db.GetTrajectoryByRoot(ctx, sess.ID)
	if n := trajNodePtr(&tr, "r:"+run.ID); n.State != db.TrajStateFailed || n.Reason != "boom" {
		t.Fatalf("flowrun node after failure = %+v", n)
	}
	if got := len(tr.Nodes); got != len(trajPhases(&tr))+1+2+1 { // phases + root + watcher/optimizer + run
		t.Fatalf("re-emitting the run must not duplicate the node: %d nodes", got)
	}
	// Transcript sessions of a triggered flow are represented by the run, not a
	// second session node.
	if trajNodePtr(&tr, "s:"+transcript.ID) != nil {
		t.Fatal("flow transcript session must not be bound as a session node")
	}
	orphan, _ := rt.db.CreateFlowRun(ctx, db.FlowRun{FlowID: flow.ID})
	rt.bindFlowRunToTrajectory(orphan)
	tr2, _ := rt.db.GetTrajectoryByRoot(ctx, sess.ID)
	if tr2.Revision != tr.Revision {
		t.Fatal("an unrelated run must not touch the trajectory")
	}
}

// TestTrajectoryForkedSessionBinding: a new root session an automation fired
// from a tree member hangs off that member with a fired edge.
func TestTrajectoryForkedSessionBinding(t *testing.T) {
	rt, base := newTrajectoryRuntime(t)
	ctx := context.Background()
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "chat", CoordinatorMode: true, CoordinatorWorkflow: "plan-dev-test"})
	if err != nil {
		t.Fatal(err)
	}
	fired, err := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "chat", Title: "Docs",
		Origin: &db.SessionOrigin{Kind: db.OriginAutomation, EntityID: "AUT9", TriggerSessionID: sess.ID}})
	if err != nil {
		t.Fatal(err)
	}
	tr, _ := rt.db.GetTrajectoryByRoot(ctx, sess.ID)
	n := trajNodePtr(&tr, "s:"+fired.ID)
	if n == nil || n.Label != "AUT9 → Docs" || n.Lane == 0 {
		t.Fatalf("fired session node = %+v", n)
	}
	var edge bool
	for _, e := range tr.Edges {
		if e.From == "s:"+sess.ID && e.To == "s:"+fired.ID && e.Kind == db.TrajEdgeFired {
			edge = true
		}
	}
	if !edge {
		t.Fatalf("edges = %+v, want fired", tr.Edges)
	}
	// The fired session is a node of the trigger's trajectory, not a root of its
	// own — even on a recipe-bearing coordinator agent (a rota-sonu watcher target).
	coordFired, err := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "chat", Title: "Retro",
		CoordinatorMode: true, CoordinatorWorkflow: "plan-dev-test",
		Origin: &db.SessionOrigin{Kind: db.OriginAutomation, EntityID: "AUT9", TriggerSessionID: sess.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.db.GetTrajectoryByRoot(ctx, coordFired.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("a session forked into a trajectory must not seed its own, got err=%v", err)
	}
	tr, _ = rt.db.GetTrajectoryByRoot(ctx, sess.ID)
	if trajNodePtr(&tr, "s:"+coordFired.ID) == nil {
		t.Fatalf("forked coordinator session missing from the trigger's trajectory: %+v", tr.Nodes)
	}
	// A trigger with no trajectory of its own (a plain chat) binds nothing, so the
	// recipe-bearing session is seeded as usual.
	plain, _ := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "chat"})
	seeded, err := rt.db.CreateSession(ctx, db.Session{AgentID: base.ID, Kind: "chat", Title: "Own",
		CoordinatorMode: true, CoordinatorWorkflow: "plan-dev-test",
		Origin: &db.SessionOrigin{Kind: db.OriginAutomation, EntityID: "AUT9", TriggerSessionID: plain.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.db.GetTrajectoryByRoot(ctx, seeded.ID); err != nil {
		t.Fatalf("a fork off a trajectory-less trigger must still be seeded: %v", err)
	}
}
