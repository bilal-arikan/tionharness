package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestAgentCoordinatorDefaultSeedsNewSession locks the whole point of putting
// coordinator mode on the AGENT: a plain new session for a coordinator agent
// arrives with the capability already on, so a template's PM/CTO never needs a
// per-thread toggle.
func TestAgentCoordinatorDefaultSeedsNewSession(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "CTO", Provider: "anthropic", Model: "m", CoordinatorMode: true,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "chat"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if !sess.IsCoordinator() {
		t.Fatal("new session of a coordinator agent should start in coordinator mode")
	}

	// The agent default must NOT leak to agents that did not ask for it.
	plain, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Solo", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create plain agent: %v", err)
	}
	ps, err := rt.db.CreateSession(ctx, db.Session{AgentID: plain.ID, Kind: "chat"})
	if err != nil {
		t.Fatalf("create plain session: %v", err)
	}
	if ps.IsCoordinator() {
		t.Fatal("an ordinary agent's session must not become a coordinator")
	}
}

// TestAgentCoordinatorDefaultDoesNotOverrideSpawner covers the exemption that
// makes the depth degrade below possible: inside a coordinator TREE the spawner
// decides, so the db-level seed must leave a deliberately-plain worker alone. If
// this regresses, SpawnWorker's degrade is silently undone one layer lower and a
// worker at the depth ceiling gets tools whose every call fails.
func TestAgentCoordinatorDefaultDoesNotOverrideSpawner(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "CTO", Provider: "anthropic", Model: "m", CoordinatorMode: true,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{
		AgentID:              a.ID,
		Kind:                 "worker",
		CoordinatorSessionID: "SES_parent",
		CoordinatorDepth:     1,
		// CoordinatorMode deliberately false — the spawner withheld it.
	})
	if err != nil {
		t.Fatalf("create worker session: %v", err)
	}
	if sess.IsCoordinator() {
		t.Fatal("a worker session inside a tree must keep the spawner's decision, not the agent default")
	}
}

// TestSpawnWorkerFoldsInAgentCoordinatorDefault verifies spawn_worker promotes a
// coordinator-by-default agent to a SUB-coordinator even when the caller omitted
// `coordinator: true` — the OR rule (the default may add the capability, never
// remove one that was asked for).
func TestSpawnWorkerFoldsInAgentCoordinatorDefault(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "CTO", Provider: "anthropic", Model: "m", CoordinatorMode: true,
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	coord := newTestCoordinator(t, rt, 0)
	defer waitWorkersSettled(t, rt, coord)

	res, err := rt.SpawnWorker(ctx, coord, "CTO", "ship the feature", "", WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	sess, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get worker session: %v", err)
	}
	if !sess.IsCoordinator() {
		t.Error("a coordinator-by-default agent spawned as a worker should become a sub-coordinator")
	}
	if !sess.IsWorker() {
		t.Error("it must still be a worker of its spawner (it owes a report upward)")
	}
}

// TestSpawnWorkerDegradesAgentDefaultAtDepthLimit is the counterpart: at the depth
// ceiling the default is dropped and the spawn SUCCEEDS as a plain worker. An
// explicit `coordinator: true` is refused there (TestSpawnWorkerDepthLimit) —
// that asymmetry is deliberate, because only the explicit form is a request the
// caller is waiting on an answer for.
func TestSpawnWorkerDegradesAgentDefaultAtDepthLimit(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.tun.SetCoordinatorLimits(0, 0, 2, 0) // CoordinatorMaxDepth = 2
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "CTO", Provider: "anthropic", Model: "m", CoordinatorMode: true,
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Parent at depth 1 → the worker lands at depth 2, which is the ceiling: a
	// sub-coordinator there could never spawn anyone.
	coord := newTestCoordinator(t, rt, 1)
	defer waitWorkersSettled(t, rt, coord)

	res, err := rt.SpawnWorker(ctx, coord, "CTO", "small fix", "", WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn should succeed as a plain worker, got: %v", err)
	}
	sess, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get worker session: %v", err)
	}
	if sess.IsCoordinator() {
		t.Error("agent coordinator default should degrade to a plain worker at the depth limit")
	}
}
