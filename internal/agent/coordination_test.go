package agent

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// TestFormatTaskNotification checks the coordinator-facing XML carries the
// worker id, status, and result.
func TestFormatTaskNotification(t *testing.T) {
	note := formatTaskNotification("SES9", "A7", "Scout", "opus-5", "completed", "found it in foo.go:42", 3, 1200)
	for _, want := range []string{
		"<task-notification>", "<task-id>SES9</task-id>", "<agent-id>A7</agent-id>",
		"<agent>Scout</agent>", "<model>opus-5</model>", "<status>completed</status>",
		"found it in foo.go:42", "<tool_uses>3</tool_uses>", "<duration_ms>1200</duration_ms>",
	} {
		if !strings.Contains(note, want) {
			t.Errorf("notification missing %q:\n%s", want, note)
		}
	}
}

// TestCountToolSteps counts only tool steps.
func TestCountToolSteps(t *testing.T) {
	steps := []TurnStep{
		{Kind: StepText}, {Kind: StepTool}, {Kind: StepThinking}, {Kind: StepTool},
	}
	if n := countToolSteps(steps); n != 2 {
		t.Fatalf("countToolSteps = %d, want 2", n)
	}
}

// TestCoordinatorQueueSerializesAndCoalesces is the critical race guard: two
// notifications that arrive while a coordinator turn is running must (a) never run
// two turns concurrently, and (b) coalesce into exactly ONE follow-up turn (they
// are already persisted, so the next turn sees them all).
func TestCoordinatorQueueSerializesAndCoalesces(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	concurrent, maxConcurrent, turns := 0, 0, 0
	started := make(chan struct{})
	release := make(chan struct{})
	rt.coordRunFn = func(string) {
		mu.Lock()
		concurrent++
		if concurrent > maxConcurrent {
			maxConcurrent = concurrent
		}
		turns++
		mu.Unlock()
		started <- struct{}{}
		<-release
		mu.Lock()
		concurrent--
		mu.Unlock()
	}

	// First enqueue starts turn 1.
	rt.enqueueCoordinatorTurn("COORD")
	<-started

	// Two more notifications arrive WHILE turn 1 runs → they must coalesce (pending),
	// not spawn new turns.
	rt.enqueueCoordinatorTurn("COORD")
	rt.enqueueCoordinatorTurn("COORD")

	// Let turn 1 finish; the pending flag triggers exactly one follow-up (turn 2).
	release <- struct{}{}
	<-started
	release <- struct{}{}

	// Give the drain goroutine a moment to settle (no third turn should appear).
	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		done := turns == 2
		mu.Unlock()
		if done {
			break
		}
		select {
		case <-deadline:
			mu.Lock()
			got := turns
			mu.Unlock()
			t.Fatalf("expected exactly 2 coordinator turns (coalesced), got %d", got)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if maxConcurrent != 1 {
		t.Fatalf("coordinator turns overlapped: maxConcurrent = %d, want 1", maxConcurrent)
	}
}

// TestUserTurnBlocksAutoTurnsAndDrainsPending: while an interactive (user) turn
// holds the coordinator slot, worker notifications must NOT start an auto turn —
// they fall into pending — and releasing the user turn must run exactly ONE
// coalesced follow-up turn.
func TestUserTurnBlocksAutoTurnsAndDrainsPending(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) {
		mu.Lock()
		turns++
		mu.Unlock()
	}

	release := rt.BeginSessionUserTurn("COORD")

	// Two notifications land mid-user-turn: no auto turn may start.
	rt.enqueueCoordinatorTurn("COORD")
	rt.enqueueCoordinatorTurn("COORD")
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if turns != 0 {
		mu.Unlock()
		t.Fatalf("auto turn ran while user turn held the slot: turns = %d", turns)
	}
	mu.Unlock()

	// Releasing the user turn drains the pile-up into exactly one turn.
	release()
	deadline := time.After(time.Second)
	for {
		mu.Lock()
		n := turns
		mu.Unlock()
		if n == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected exactly 1 coalesced turn after release, got %d", n)
		case <-time.After(10 * time.Millisecond):
		}
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if turns != 1 {
		t.Fatalf("expected exactly 1 coalesced turn, got %d", turns)
	}
}

// TestUserTurnWaitsForAutoTurnAndResetsCap: a user turn arriving while an auto
// turn runs must block until it finishes, and claiming the slot resets the
// auto-turn cap (human back in the loop).
func TestUserTurnWaitsForAutoTurnAndResetsCap(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	started := make(chan struct{})
	releaseAuto := make(chan struct{})
	rt.coordRunFn = func(string) {
		started <- struct{}{}
		<-releaseAuto
	}

	// Pre-load cap state to verify the reset.
	slot := rt.coordSlotFor("COORD")
	slot.mu.Lock()
	slot.turns = 40
	slot.capWarn = true
	slot.mu.Unlock()

	rt.enqueueCoordinatorTurn("COORD")
	<-started

	acquired := make(chan func(), 1)
	go func() { acquired <- rt.BeginSessionUserTurn("COORD") }()

	select {
	case <-acquired:
		t.Fatal("user turn acquired the slot while an auto turn was running")
	case <-time.After(50 * time.Millisecond):
	}

	releaseAuto <- struct{}{}
	var release func()
	select {
	case release = <-acquired:
	case <-time.After(time.Second):
		t.Fatal("user turn never acquired the slot after the auto turn finished")
	}

	slot.mu.Lock()
	turnsAfter, warnAfter := slot.turns, slot.capWarn
	slot.mu.Unlock()
	if turnsAfter != 0 || warnAfter {
		t.Fatalf("user turn should reset cap state, got turns=%d capWarn=%v", turnsAfter, warnAfter)
	}
	release()
}

// TestClaimSessionTurnSlot: an autonomous claim (wake/scheduled/peer) blocks auto
// turns into pending WITHOUT resetting the coordinator cap — for a coordinator
// session AND for a plain session, which now serializes exactly the same way (the
// old coordinator-only gate that returned a no-op for plain sessions is gone).
func TestClaimSessionTurnSlot(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	// Slot is claimed for an autonomous turn — a notification must fall into
	// pending, and the cap state must survive (autonomous claim does not reset it).
	slot := rt.coordSlotFor("COORD")
	slot.mu.Lock()
	slot.turns = 7
	slot.mu.Unlock()
	release := rt.claimSessionTurnSlot("COORD", turnqueue.KindWake, "uyandırma")
	rt.enqueueCoordinatorTurn("COORD")
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if turns != 0 {
		mu.Unlock()
		t.Fatalf("auto turn ran while autonomous turn held the slot: turns = %d", turns)
	}
	mu.Unlock()
	slot.mu.Lock()
	if slot.turns != 7 {
		slot.mu.Unlock()
		t.Fatalf("autonomous claim must not reset the auto-turn cap, turns = %d", slot.turns)
	}
	slot.mu.Unlock()
	release()
}

// TestPlainSessionSerializesConcurrentTurns is the #1 regression guard (_Docs/58):
// on a PLAIN (non-coordinator) session, a second turn-entry path must block on the
// per-session slot until the first releases — this is the wake-vs-user /
// direct-chat-vs-inbox-worker race that the old coordinator-only gate left open.
func TestPlainSessionSerializesConcurrentTurns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	// A user turn (e.g. inbox worker) holds the slot for a plain session.
	release := rt.BeginSessionUserTurn("PLAIN")

	// A concurrent autonomous turn (e.g. a scheduler wake re-entering this chat
	// session) tries to claim the same slot — it MUST block until release.
	acquired := make(chan func(), 1)
	go func() { acquired <- rt.claimSessionTurnSlot("PLAIN", turnqueue.KindWake, "uyandırma") }()

	select {
	case <-acquired:
		t.Fatal("autonomous turn acquired a plain session's slot while a user turn held it")
	case <-time.After(50 * time.Millisecond):
	}

	// Releasing the user turn lets the waiting autonomous turn proceed.
	release()
	select {
	case wakeRelease := <-acquired:
		wakeRelease()
	case <-time.After(time.Second):
		t.Fatal("autonomous turn never acquired the slot after the user turn released")
	}
}

// TestSpawnWorkerRespectsWorkerCap verifies the per-coordinator worker cap refuses
// a spawn once the active-worker count is at the limit.
func TestSpawnWorkerRespectsWorkerCap(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.tun.SetCoordinatorLimits(2, 0, 0, 0)
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	coord := newTestCoordinator(t, rt, 0)
	// Simulate two workers already active under this coordinator.
	rt.coordSlotFor(coord).workers.Add(2)

	if _, err := rt.SpawnWorker(ctx, coord, "W", "task", "", WorkerSpec{}); err == nil {
		t.Fatal("expected worker-limit error when the cap is already reached")
	}
}

// waitWorkersSettled blocks until no worker turn or notification drain under
// coordID is still running.
// Spawns are fire-and-forget, so without this a test can return while a detached
// runWorker/drainCoordinator goroutine is still writing into the session store —
// which then fails the t.TempDir cleanup with "directory not empty" rather than
// in the assertion.
func waitWorkersSettled(t *testing.T, rt *Runtime, coordIDs ...string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		busy := false
		for _, id := range coordIDs {
			slot := rt.coordSlotFor(id)
			slot.mu.Lock()
			driving := slot.driving
			slot.mu.Unlock()
			if slot.workers.Load() > 0 || driving {
				busy = true
			}
		}
		if !busy {
			return
		}
		select {
		case <-deadline:
			t.Fatal("worker turns did not settle in time")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// newTestCoordinator creates a coordinator session at the given tree depth and
// returns its id. Depth is stamped directly (rather than by spawning a chain) so a
// test can exercise the depth guard without building the levels above it.
func newTestCoordinator(t *testing.T, rt *Runtime, depth int) string {
	t.Helper()
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Coord" + strconv.Itoa(depth), Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create coordinator agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{
		AgentID:          a.ID,
		Kind:             "chat",
		SourceID:         "test:coord:" + strconv.Itoa(depth),
		CoordinatorMode:  true,
		CoordinatorDepth: depth,
	})
	if err != nil {
		t.Fatalf("create coordinator session: %v", err)
	}
	return sess.ID
}

// seedSystemAgents installs the canonical built-ins into a test runtime's store,
// mirroring what workspace boot does. Profile workers now resolve to those agents,
// so a runtime without them cannot spawn one.
func seedSystemAgents(t *testing.T, rt *Runtime) {
	t.Helper()
	if err := rt.db.EnsureSystemAgents(context.Background(), SystemAgentDefaults()...); err != nil {
		t.Fatalf("seed system agents: %v", err)
	}
}

// TestSpawnWorkerTargetsSystemAgent verifies a profile target (explore) resolves
// to the shared "subagent-explore" SYSTEM agent — reused across spawns and never
// materialized into a per-profile "worker:<id>" copy.
func TestSpawnWorkerTargetsSystemAgent(t *testing.T) {
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
		t.Fatalf("spawn worker (explore): %v", err)
	}
	s1, _ := rt.db.GetSession(ctx, r1.SessionID)
	sys, ok := rt.db.FindAgentBySystemKey("subagent-explore")
	if !ok {
		t.Fatal("subagent-explore system agent missing")
	}
	if s1.AgentID != sys.ID {
		t.Errorf("worker session agent = %q, want system agent %q", s1.AgentID, sys.ID)
	}

	// Second spawn reuses the same system agent and creates no extra rows.
	if _, err := rt.SpawnWorker(ctx, coord, "explore", "again", base.ID, WorkerSpec{}); err != nil {
		t.Fatalf("second spawn: %v", err)
	}
	agents, _ := rt.db.ListAgents(ctx)
	n := 0
	for _, a := range agents {
		if a.Name == "worker:explore" || a.SystemKey == "subagent-explore" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected exactly one explore worker agent, got %d", n)
	}
}

// TestResolveWorkerTargetReassertsProfileAllowlist proves the allowlist is a code
// contract: an edited system-agent row is pulled back to the profile's tools,
// while unrelated per-agent customization (tool overrides) survives.
func TestResolveWorkerTargetReassertsProfileAllowlist(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	seedSystemAgents(t, rt)
	sys, ok := rt.db.FindAgentBySystemKey("subagent-validator")
	if !ok {
		t.Fatal("subagent-validator system agent missing")
	}
	if err := rt.db.UpdateAgentAllowedTools(ctx, sys.ID, `["Read","Write","Bash"]`); err != nil {
		t.Fatal(err)
	}
	if err := rt.db.UpdateAgentTools(ctx, sys.ID, true, `{"Write":"blocked"}`); err != nil {
		t.Fatal(err)
	}

	id, err := rt.resolveWorkerTarget(ctx, "", "", "validator")
	if err != nil {
		t.Fatal(err)
	}
	if id != sys.ID {
		t.Fatalf("resolveWorkerTarget = %q, want system agent %q", id, sys.ID)
	}
	got, _ := rt.db.GetAgent(ctx, sys.ID)
	if got.AllowedTools != mustJSON(t, defaultSubagentProfiles["validator"].AllowedTools) {
		t.Fatalf("edited allowlist not re-asserted: %s", got.AllowedTools)
	}
	if got.ToolOverrides != `{"Write":"blocked"}` {
		t.Fatalf("custom overrides overwritten: %s", got.ToolOverrides)
	}
}

// TestApplyProfileAllowlistIgnoresNonProfileAgents keeps the re-assertion scoped:
// only a subagent-* system agent is rewritten from code.
func TestApplyProfileAllowlistIgnoresNonProfileAgents(t *testing.T) {
	plain := db.Agent{Name: "Coord", AllowedTools: `["Read"]`}
	if err := applyProfileAllowlist(&plain); err != nil {
		t.Fatal(err)
	}
	if plain.AllowedTools != `["Read"]` {
		t.Fatalf("non-system agent rewritten: %s", plain.AllowedTools)
	}
	other := db.Agent{Name: "Titler", System: true, SystemKey: "titler", AllowedTools: `[]`}
	if err := applyProfileAllowlist(&other); err != nil {
		t.Fatal(err)
	}
	if other.AllowedTools != `[]` {
		t.Fatalf("non-profile system agent rewritten: %s", other.AllowedTools)
	}
	worker := db.Agent{Name: "Worker: Coder", System: true, SystemKey: "subagent-coder", AllowedTools: `["Bash"]`}
	if err := applyProfileAllowlist(&worker); err != nil {
		t.Fatal(err)
	}
	if worker.AllowedTools != mustJSON(t, defaultSubagentProfiles["coder"].AllowedTools) {
		t.Fatalf("profile worker allowlist = %s", worker.AllowedTools)
	}
}

// TestSpawnWorkerSetsCoordinatorLink confirms a worker spawn creates a worker-kind
// session linked back to its coordinator.
func TestSpawnWorkerSetsCoordinatorLink(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	coord := newTestCoordinator(t, rt, 0)
	res, err := rt.SpawnWorker(ctx, coord, "W", "do it", "", WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	defer waitWorkersSettled(t, rt, coord)
	sess, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Kind != "worker" {
		t.Errorf("kind = %q, want worker", sess.Kind)
	}
	if sess.Role != "worker" {
		t.Errorf("role = %q, want worker", sess.Role)
	}
	if sess.CoordinatorSessionID != coord {
		t.Errorf("coordinatorSessionID = %q, want %q", sess.CoordinatorSessionID, coord)
	}
	// Tree placement is stamped at creation, so the depth/subtree guards and the
	// tree endpoints can trust it without walking parent links.
	if sess.CoordinatorDepth != 1 {
		t.Errorf("coordinatorDepth = %d, want 1", sess.CoordinatorDepth)
	}
	if sess.RootCoordinatorSessionID != coord {
		t.Errorf("root = %q, want %q", sess.RootCoordinatorSessionID, coord)
	}
	if sess.IsCoordinator() {
		t.Error("a plain worker must not get coordinator mode")
	}
}

// TestSpawnSubCoordinatorNests verifies the whole point of the depth rework: a
// worker spawned with Coordinator:true gets coordinator mode WITHOUT losing its
// worker lineage, and can spawn a worker of its own one level deeper.
func TestSpawnSubCoordinatorNests(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	root := newTestCoordinator(t, rt, 0)

	mid, err := rt.SpawnWorker(ctx, root, "W", "split this", "", WorkerSpec{Coordinator: true})
	defer func() { waitWorkersSettled(t, rt, root, mid.SessionID) }()
	if err != nil {
		t.Fatalf("spawn sub-coordinator: %v", err)
	}
	midSess, _ := rt.db.GetSession(ctx, mid.SessionID)
	if !midSess.IsCoordinator() {
		t.Fatal("sub-coordinator must have coordinator mode")
	}
	if !midSess.IsWorker() {
		t.Fatal("sub-coordinator must still be a worker of its parent")
	}

	leaf, err := rt.SpawnWorker(ctx, mid.SessionID, "W", "the actual work", "", WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn under sub-coordinator: %v", err)
	}
	leafSess, _ := rt.db.GetSession(ctx, leaf.SessionID)
	if leafSess.CoordinatorDepth != 2 {
		t.Errorf("leaf depth = %d, want 2", leafSess.CoordinatorDepth)
	}
	// Every level shares ONE root, which is what keeps the tree-wide budget and the
	// shared scratchpad path from fragmenting by level.
	if leafSess.RootCoordinatorSessionID != root {
		t.Errorf("leaf root = %q, want %q", leafSess.RootCoordinatorSessionID, root)
	}

	tree, err := rt.db.ListCoordinatorTree(ctx, leaf.SessionID)
	if err != nil {
		t.Fatalf("list tree from a leaf: %v", err)
	}
	if len(tree) != 3 {
		t.Fatalf("tree size = %d, want 3 (root + mid + leaf)", len(tree))
	}
	if tree[0].ID != root {
		t.Errorf("tree[0] = %q, want the root %q (a leaf's id must normalize to the root)", tree[0].ID, root)
	}
}

// TestSpawnWorkerDepthLimit verifies the depth guard REFUSES a sub-coordinator
// that would not have room for its own workers, instead of silently downgrading
// it to a leaf — a coordinator that believes it delegated work it did not
// delegate would wait forever for a report.
func TestSpawnWorkerDepthLimit(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.tun.SetCoordinatorLimits(0, 0, 2, 0) // max depth 2
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// A coordinator already at depth 1: its workers land at depth 2 (allowed as
	// leaves) but a sub-coordinator there would need a depth-3 level.
	coord := newTestCoordinator(t, rt, 1)
	defer waitWorkersSettled(t, rt, coord)

	if _, err := rt.SpawnWorker(ctx, coord, "W", "leaf work", "", WorkerSpec{}); err != nil {
		t.Fatalf("a plain worker at the last allowed depth must be accepted: %v", err)
	}
	if _, err := rt.SpawnWorker(ctx, coord, "W", "split further", "", WorkerSpec{Coordinator: true}); err == nil {
		t.Fatal("expected a sub-coordinator at the depth limit to be refused")
	}
}

// TestSpawnWorkerSubtreeBudget verifies the tree-wide budget counts only LIVE
// workers and reclaims finished ones. The budget bounds concurrent fan-out (the
// actual explosion vector); a worker that concluded, failed, or was stopped is
// reclaimed so a long-running coordinator is not permanently bricked by the
// sessions of work it already finished. The exhaustion error also names which
// workers still hold the budget.
func TestSpawnWorkerSubtreeBudget(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.tun.SetCoordinatorLimits(0, 0, 0, 2) // whole tree: 2 LIVE worker sessions
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	root := newTestCoordinator(t, rt, 0)

	// Seed the tree to capacity with two workers that are genuinely live (their turn
	// markers are set). newTreeNode creates the sessions without running a turn, so
	// liveness is fully under the test's control.
	w1 := newTreeNode(t, rt, "w1", root, root, 1, false)
	w2 := newTreeNode(t, rt, "w2", root, root, 1, false)
	rt.trackSession(w1.ID, func() {})
	rt.trackSession(w2.ID, func() {})

	// A third worker is refused, and the error names both live workers holding the
	// budget — the diagnostic a coordinator needs to decide what to conclude.
	_, err := rt.SpawnWorker(ctx, root, "W", "three", "", WorkerSpec{})
	if err == nil {
		rt.untrackSession(w1.ID)
		rt.untrackSession(w2.ID)
		t.Fatal("expected the tree-wide budget to refuse a spawn while two workers are live")
	}
	if !strings.Contains(err.Error(), "exhausted") || !strings.Contains(err.Error(), "2/2") {
		t.Errorf("error should report the exhausted budget, got: %v", err)
	}
	if !strings.Contains(err.Error(), w1.ID) || !strings.Contains(err.Error(), w2.ID) {
		t.Errorf("error should list the live workers holding the budget, got: %v", err)
	}

	// One worker finishes: its slot must be reclaimed so the coordinator can spawn
	// again — the behavior the exhaustion message has always promised.
	rt.untrackSession(w1.ID)
	defer func() {
		rt.untrackSession(w2.ID)
		waitWorkersSettled(t, rt, root)
	}()
	res, err := rt.SpawnWorker(ctx, root, "W", "after-reclaim", "", WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn after a worker was reclaimed should succeed: %v", err)
	}
	// Visibility: the result reports the tree now sits at 2/2 (the still-live w2 plus
	// the worker just spawned).
	if res.TreeBudgetTotal != 2 || res.TreeBudgetUsed != 2 {
		t.Errorf("spawn result budget = %d/%d, want 2/2", res.TreeBudgetUsed, res.TreeBudgetTotal)
	}
}

// waitTurns blocks until *turns reaches want or the deadline elapses, guarding the
// counter with mu. Fails the test on timeout.
func waitTurns(t *testing.T, mu *sync.Mutex, turns *int, want int, what string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		got := *turns
		mu.Unlock()
		if got == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("%s: expected %d turns, got %d", what, want, got)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestIdleReconcileSweepRunsFinalTurn: when every worker has finished (workers==0)
// under a coordinator that actually spawned workers, the drain loop must run ONE
// extra authoritative reconcile turn — the liveness backstop against a coalesced
// notification the model overlooked. It must be one-shot per all-idle transition
// and re-arm on the next notification.
func TestIdleReconcileSweepRunsFinalTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	slot := rt.coordSlotFor("COORD")
	slot.markHadWorkers() // simulate a coordinator that has spawned worker(s), now all done

	// One notification: the processing turn (1) plus the idle-reconcile turn (2).
	rt.enqueueCoordinatorTurn("COORD")
	waitTurns(t, &mu, &turns, 2, "process + reconcile")

	// One-shot: no further idle turn may appear without a new notification.
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	got := turns
	mu.Unlock()
	if got != 2 {
		t.Fatalf("idle sweep must be one-shot; got %d turns", got)
	}

	// A new notification re-arms the sweep → process (3) + reconcile (4).
	rt.enqueueCoordinatorTurn("COORD")
	waitTurns(t, &mu, &turns, 4, "re-armed process + reconcile")
}

// TestUserStopSkipsIdleReconcile: when the user presses Stop on a coordinator's live
// turn (runCoordinatorTurn sets slot.stopRequested), the drain loop must EXIT without
// the idle-reconcile turn — even for a coordinator whose workers all finished, where
// the sweep would otherwise inject a <coordination-status> note and run once more. The
// bug this guards: a manual Stop looked like the session "kept going".
func TestUserStopSkipsIdleReconcile(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	slot := rt.coordSlotFor("COORD")
	slot.markHadWorkers() // all workers done → the idle sweep WOULD normally fire

	var mu sync.Mutex
	turns := 0
	// Simulate the user pressing Stop during the first turn: runCoordinatorTurn sets
	// stopRequested when the turn ends on context.Canceled.
	rt.coordRunFn = func(string) {
		mu.Lock()
		turns++
		mu.Unlock()
		slot.mu.Lock()
		slot.stopRequested = true
		slot.mu.Unlock()
	}

	rt.enqueueCoordinatorTurn("COORD")
	waitTurns(t, &mu, &turns, 1, "single stopped turn")

	// No idle-reconcile turn may follow the stop, and the flag must be consumed.
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	got := turns
	mu.Unlock()
	if got != 1 {
		t.Fatalf("user Stop must skip idle-reconcile; got %d turns", got)
	}
	slot.mu.Lock()
	stillSet, pending, driving := slot.stopRequested, slot.pending, slot.driving
	slot.mu.Unlock()
	if stillSet {
		t.Fatal("stopRequested must be consumed (one-shot) by the drain loop")
	}
	if pending || driving {
		t.Fatalf("drain loop must exit clean after a Stop; pending=%v driving=%v", pending, driving)
	}
	if rt.sessionTurnBusy("COORD") {
		t.Fatal("the admission slot must be free once the drain exits")
	}

	// A later worker notification must still re-arm the loop (Stop is not permanent):
	// process (2) + idle-reconcile (3), since stopRequested is no longer set.
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }
	rt.enqueueCoordinatorTurn("COORD")
	waitTurns(t, &mu, &turns, 3, "re-armed process + reconcile after stop")
}

// TestUserStopHonoursPendingWorker: a Stop must suppress ONLY the phantom idle-reconcile
// turn. If a worker notification is pending when the user stops (a worker finished, or is
// still running), that notification must still supersede the Stop and run a turn — a
// running worker continues to drive the coordinator through a Stop.
func TestUserStopHonoursPendingWorker(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	slot := rt.coordSlotFor("COORD")
	slot.markHadWorkers()
	slot.workers.Add(1) // a worker is still running through the Stop

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) {
		mu.Lock()
		n := turns
		turns++
		mu.Unlock()
		if n == 0 {
			// The user stops the live turn AND a worker notification lands in the same window.
			slot.mu.Lock()
			slot.stopRequested = true
			slot.pending = true
			slot.mu.Unlock()
		}
	}

	rt.enqueueCoordinatorTurn("COORD")
	// The pending worker notification supersedes the Stop → a 2nd turn runs.
	waitTurns(t, &mu, &turns, 2, "pending worker turn must survive a Stop")

	// No idle-reconcile while a worker is still running (workers>0), and no extra turn.
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	got := turns
	mu.Unlock()
	if got != 2 {
		t.Fatalf("expected exactly 2 turns (no idle-reconcile while a worker runs), got %d", got)
	}
}

// TestIdleReconcileSkippedWithoutWorkers: a coordinator that never spawned a worker
// must NOT get an idle-reconcile turn (nothing to reconcile) — a single notification
// yields exactly one turn.
func TestIdleReconcileSkippedWithoutWorkers(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	rt.enqueueCoordinatorTurn("COORD")
	waitTurns(t, &mu, &turns, 1, "single turn, no reconcile")
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	got := turns
	mu.Unlock()
	if got != 1 {
		t.Fatalf("no-worker coordinator must not reconcile; got %d turns", got)
	}
}
