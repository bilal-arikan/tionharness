package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// newTreeNode creates a session at a given place in a coordinator tree without
// running any turn, so the tree-shape logic can be tested in isolation.
func newTreeNode(t *testing.T, rt *Runtime, id, parent, root string, depth int, coordinator bool) db.Session {
	t.Helper()
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: id, Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	role := ""
	if parent != "" {
		role = db.SessionRoleWorker
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{
		AgentID:                  a.ID,
		Kind:                     "worker",
		SourceID:                 "test:" + id,
		Title:                    id,
		Role:                     role,
		CoordinatorMode:          coordinator,
		CoordinatorSessionID:     parent,
		RootCoordinatorSessionID: root,
		CoordinatorDepth:         depth,
	})
	if err != nil {
		t.Fatalf("create session %s: %v", id, err)
	}
	return sess
}

// TestListSubtreeWorkersRerootsAtCaller verifies a mid-level node sees only what is
// genuinely BELOW it. ListCoordinatorTree normalizes to the tree root, so without
// re-rooting a sub-coordinator would be handed its parent's other branches — work
// it neither owns nor may act on.
func TestListSubtreeWorkersRerootsAtCaller(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	root := newTreeNode(t, rt, "root", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid", root.ID, root.ID, 1, true)
	leafA := newTreeNode(t, rt, "leafA", mid.ID, root.ID, 2, false)
	sibling := newTreeNode(t, rt, "sibling", root.ID, root.ID, 1, false)

	fromRoot, err := rt.ListSubtreeWorkers(ctx, root.ID)
	if err != nil {
		t.Fatalf("subtree from root: %v", err)
	}
	if len(fromRoot) != 3 {
		t.Errorf("root subtree = %d nodes, want 3 (mid + leafA + sibling)", len(fromRoot))
	}

	fromMid, err := rt.ListSubtreeWorkers(ctx, mid.ID)
	if err != nil {
		t.Fatalf("subtree from mid: %v", err)
	}
	if len(fromMid) != 1 || fromMid[0].SessionID != leafA.ID {
		t.Fatalf("mid subtree = %+v, want only leafA", fromMid)
	}
	if fromMid[0].Depth != 1 {
		t.Errorf("depth is relative to the caller: got %d, want 1", fromMid[0].Depth)
	}
	for _, w := range fromMid {
		if w.SessionID == sibling.ID {
			t.Error("a sub-coordinator must not see its parent's other branches")
		}
	}
}

// TestDeferWorkerReportWithholdsWhileDelegating is the core semantic guard: a
// sub-coordinator whose own workers are still running must NOT have its finished
// turn reported upward as a result, or its coordinator concludes on top of a
// branch that has barely started.
func TestDeferWorkerReportWithholdsWhileDelegating(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	root := newTreeNode(t, rt, "root", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid", root.ID, root.ID, 1, true)
	leaf := newTreeNode(t, rt, "leaf", mid.ID, root.ID, 2, false)

	// Nothing running yet: the mid node's turn IS its result, report it normally.
	if rt.deferWorkerReport(ctx, mid, turnStatusCompleted) {
		t.Error("a sub-coordinator with no live workers must report normally")
	}

	// Its own worker is now in flight (both the slot counter and the live session
	// set, mirroring what SpawnWorker + runWorker do).
	rt.coordSlotFor(mid.ID).workers.Add(1)
	rt.trackSession(leaf.ID, func() {})
	defer rt.untrackSession(leaf.ID)

	if !rt.deferWorkerReport(ctx, mid, turnStatusCompleted) {
		t.Fatal("a sub-coordinator with running workers must NOT report completion yet")
	}
	if !rt.owesReportNow(ctx, mid.ID) {
		t.Error("withholding the report must record that one is still owed")
	}

	// A FAILED turn is different: the branch is broken, so the parent must hear
	// about it immediately rather than waiting for a synthesis that will not come.
	if rt.deferWorkerReport(ctx, mid, turnStatusFailed) {
		t.Error("a failed sub-coordinator must report immediately")
	}

	// A leaf worker is never deferred — its turn is the whole of its work.
	if rt.deferWorkerReport(ctx, leaf, turnStatusCompleted) {
		t.Error("a plain worker's turn must always report")
	}
}

// TestReportToCoordinatorRejectsPrematureCompletion verifies a sub-coordinator
// cannot claim "completed" while its own workers are still running — that claim
// would be a lie the coordinator above has no way to detect.
func TestReportToCoordinatorRejectsPrematureCompletion(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	root := newTreeNode(t, rt, "root", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid", root.ID, root.ID, 1, true)
	leaf := newTreeNode(t, rt, "leaf", mid.ID, root.ID, 2, false)

	rt.trackSession(leaf.ID, func() {})
	err := rt.ReportToCoordinator(ctx, mid.ID, turnStatusCompleted, "all done!")
	rt.untrackSession(leaf.ID)
	if err == nil {
		t.Fatal("expected a premature \"completed\" report to be refused")
	}
	if !strings.Contains(err.Error(), "still running") {
		t.Errorf("error should name the reason, got: %v", err)
	}

	// With the branch settled the report goes through and lands in the coordinator's
	// history as a task-notification.
	if err := rt.ReportToCoordinator(ctx, mid.ID, turnStatusCompleted, "synthesis of both workers"); err != nil {
		t.Fatalf("report after settling: %v", err)
	}
	// Find the report by content, not position: ReportToCoordinator enqueues the
	// coordinator's own turn, whose drain goroutine may append further messages to
	// root asynchronously — so the note is not reliably the LAST message (a loaded
	// runner surfaces this as a flake).
	msgs, err := rt.db.ListMessages(ctx, root.ID)
	if err != nil {
		t.Fatalf("list coordinator messages: %v", err)
	}
	var note *db.Message
	for i := range msgs {
		if strings.Contains(msgs[i].Text, "synthesis of both workers") {
			note = &msgs[i]
			break
		}
	}
	if note == nil {
		t.Fatalf("coordinator never received the settled report among %d messages", len(msgs))
	}
	if !strings.Contains(note.Text, "<task-notification>") {
		t.Errorf("report should be a task-notification:\n%s", note.Text)
	}
	if note.Origin != "worker-note" {
		t.Errorf("origin = %q, want worker-note (so the UI renders it as a worker report)", note.Origin)
	}
}

// TestSetCoordinatorModeRefusesOffWithRunningWorkers verifies the self-service
// toggle will not strip an agent of the tools it needs to manage workers that are
// still running — the notifications keep arriving either way.
func TestSetCoordinatorModeRefusesOffWithRunningWorkers(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	coord := newTreeNode(t, rt, "coord", "", "", 0, true)
	leaf := newTreeNode(t, rt, "leaf", coord.ID, coord.ID, 1, false)

	rt.trackSession(leaf.ID, func() {})
	_, err := rt.SetSessionCoordinatorMode(ctx, coord.ID, false)
	rt.untrackSession(leaf.ID)
	if err == nil {
		t.Fatal("expected turning coordinator mode off to be refused while a worker runs")
	}

	if _, err := rt.SetSessionCoordinatorMode(ctx, coord.ID, false); err != nil {
		t.Fatalf("turning it off once settled: %v", err)
	}
	got, _ := rt.db.GetSession(ctx, coord.ID)
	if got.IsCoordinator() {
		t.Error("coordinator mode should be off after the toggle")
	}

	// And back on again — an ordinary session can promote itself.
	if _, err := rt.SetSessionCoordinatorMode(ctx, coord.ID, true); err != nil {
		t.Fatalf("turning it back on: %v", err)
	}
	got, _ = rt.db.GetSession(ctx, coord.ID)
	if !got.IsCoordinator() {
		t.Error("coordinator mode should be on after re-enabling")
	}
}

// TestSetCoordinatorModeRespectsDepthLimit verifies a session at the depth limit
// is refused the capability outright, instead of being handed tools whose every
// call would fail (which reads as a broken tool, not a boundary).
func TestSetCoordinatorModeRespectsDepthLimit(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.tun.SetCoordinatorLimits(0, 0, 2, 0)
	ctx := context.Background()

	root := newTreeNode(t, rt, "root", "", "", 0, true)
	deep := newTreeNode(t, rt, "deep", root.ID, root.ID, 2, false) // already at the limit

	if _, err := rt.SetSessionCoordinatorMode(ctx, deep.ID, true); err == nil {
		t.Fatal("expected a session at the depth limit to be refused coordinator mode")
	}
}

// TestCoordinationFuncsGating verifies the per-tool gate: the worker-driving tools
// only for a coordinator, report_to_coordinator only for a session with a parent,
// set_coordinator_mode always (else a plain session could never opt in).
func TestCoordinationFuncsGating(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	plain := db.Session{ID: "S1"}
	f := rt.coordinationFuncsFor(plain, "AGT1")
	if f.Spawn != nil || f.Report != nil {
		t.Error("a plain session must get neither the worker tools nor report_to_coordinator")
	}
	if f.SetMode == nil {
		t.Error("every session must be able to turn coordinator mode on")
	}

	rootCoord := db.Session{ID: "S2", CoordinatorMode: true}
	f = rt.coordinationFuncsFor(rootCoord, "AGT1")
	if f.Spawn == nil || f.List == nil {
		t.Error("a coordinator must get the worker-driving tools")
	}
	if f.Report != nil {
		t.Error("a root coordinator has nothing above it to report to")
	}

	mid := db.Session{ID: "S3", Role: db.SessionRoleWorker, CoordinatorMode: true, CoordinatorSessionID: "S2"}
	f = rt.coordinationFuncsFor(mid, "AGT1")
	if f.Spawn == nil {
		t.Error("a mid-level node drives its own workers — this is what unlocks depth")
	}
	if f.Report == nil {
		t.Error("a mid-level node owes its coordinator a report")
	}

	// The CLI bridge advertises exactly the same subset, so a claude-cli turn and a
	// native turn never disagree about which tools exist.
	names := map[string]bool{}
	for _, d := range coordinationBridgeDefs(f) {
		names[d.Name] = true
	}
	for _, want := range []string{"spawn_worker", "report_to_coordinator", "set_coordinator_mode"} {
		if !names[want] {
			t.Errorf("bridge defs missing %q for a mid-level node", want)
		}
	}
}

// TestLegacyCoordinatorSessionStillWorks verifies sessions written by older builds
// (Role="coordinator", no CoordinatorMode flag) keep their capability without a
// migration, and that turning the mode off actually demotes them.
func TestLegacyCoordinatorSessionStillWorks(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "L", Provider: "anthropic", Model: "m"})
	legacy, err := rt.db.CreateSession(ctx, db.Session{
		AgentID: a.ID, Kind: "chat", SourceID: "test:legacy",
		Role: db.SessionRoleCoordinator, // the old way of saying "coordinator"
	})
	if err != nil {
		t.Fatalf("create legacy session: %v", err)
	}
	if !legacy.IsCoordinator() {
		t.Fatal("a legacy Role=coordinator session must still read as a coordinator")
	}
	if legacy.RootCoordinator() != legacy.ID {
		t.Errorf("a legacy coordinator is its own tree root, got %q", legacy.RootCoordinator())
	}

	if _, err := rt.SetSessionCoordinatorMode(ctx, legacy.ID, false); err != nil {
		t.Fatalf("demote legacy coordinator: %v", err)
	}
	got, _ := rt.db.GetSession(ctx, legacy.ID)
	if got.IsCoordinator() {
		t.Error("demoting must also clear the legacy Role value, else the toggle is a no-op")
	}
}

// TestOwesReportSurvivesRestart is the regression guard for the persisted
// pending-report flag. A mid-level node waiting on its branch looks HEALTHY on
// disk — its last message is its own assistant reply — so orphan recovery never
// touches it. With the flag in memory only, a restart forgot the owed report and
// its coordinator waited forever.
func TestOwesReportSurvivesRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "workspace")
	rt, _ := newTestRuntime(t, dir)
	ctx := context.Background()

	root := newTreeNode(t, rt, "root", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid", root.ID, root.ID, 1, true)
	leaf := newTreeNode(t, rt, "leaf", mid.ID, root.ID, 2, false)

	rt.coordSlotFor(mid.ID).workers.Add(1)
	rt.trackSession(leaf.ID, func() {})
	if !rt.deferWorkerReport(ctx, mid, turnStatusCompleted) {
		t.Fatal("expected the report to be withheld while the branch is live")
	}
	rt.untrackSession(leaf.ID)

	// The flag is on the session, not on a goroutine's memory.
	got, _ := rt.db.GetSession(ctx, mid.ID)
	if !got.CoordinatorReportPending {
		t.Fatal("withholding a report must persist that one is owed")
	}
	pending, err := rt.db.ListPendingCoordinatorReports(ctx)
	if err != nil || len(pending) != 1 || pending[0].ID != mid.ID {
		t.Fatalf("boot scan should find exactly the mid node, got %+v (err=%v)", pending, err)
	}

	// Reporting clears it, so a restart does not re-report work already delivered.
	if err := rt.ReportToCoordinator(ctx, mid.ID, turnStatusCompleted, "done"); err != nil {
		t.Fatalf("report: %v", err)
	}
	got, _ = rt.db.GetSession(ctx, mid.ID)
	if got.CoordinatorReportPending {
		t.Error("delivering the report must clear the pending flag")
	}
	if pending, _ := rt.db.ListPendingCoordinatorReports(ctx); len(pending) != 0 {
		t.Errorf("nothing should still be owed, got %d", len(pending))
	}
}

// TestSettleBackstopReportsIncomplete verifies the backstop hands SOMETHING
// upward for a node that went quiet without reporting — and that it never claims
// "completed", because the runtime cannot know the work actually finished.
func TestSettleBackstopReportsIncomplete(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	root := newTreeNode(t, rt, "root", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid", root.ID, root.ID, 1, true)
	if _, err := rt.db.AddMessage(ctx, db.Message{
		SessionID: mid.ID, AgentID: mid.AgentID, Role: "assistant", Text: "iki worker açtım",
	}); err != nil {
		t.Fatalf("seed reply: %v", err)
	}
	rt.setOwesReport(ctx, mid.ID, true)

	rt.settleReportBackstop(ctx, mid.ID)

	// Locate the backstop note by its content, not by position: NotifyCoordinator
	// enqueues the coordinator's own turn, whose drain goroutine may append further
	// messages to root asynchronously — so the note is not reliably the LAST message
	// (a loaded runner surfaces this as a flake). The note is the one carrying the
	// node's seeded reply.
	countBackstops := func() (int, string) {
		msgs, err := rt.db.ListMessages(ctx, root.ID)
		if err != nil {
			t.Fatalf("list root messages: %v", err)
		}
		n, last := 0, ""
		for _, m := range msgs {
			if strings.Contains(m.Text, "iki worker açtım") {
				n++
				last = m.Text
			}
		}
		return n, last
	}

	got, note := countBackstops()
	if got == 0 {
		t.Fatalf("coordinator got no backstop note carrying the node's reply")
	}
	if !strings.Contains(note, "<status>incomplete</status>") {
		t.Errorf("backstop must not claim completion:\n%s", note)
	}
	// One-shot: a second sweep must not re-report — the backstop-note count stays put.
	rt.settleReportBackstop(ctx, mid.ID)
	if again, _ := countBackstops(); again != got {
		t.Errorf("backstop fired twice: %d → %d notes", got, again)
	}
}

// TestCoordinatorSettleGraceIsConfigurable verifies the backstop delay follows the
// setting (and falls back to the default), since the right value depends entirely
// on how long the coordinators' model takes to synthesize.
func TestCoordinatorSettleGraceIsConfigurable(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	if got, want := rt.tun.CoordinatorSettleGrace(), time.Duration(DefaultCoordinatorSettleGraceSec)*time.Second; got != want {
		t.Errorf("default grace = %v, want %v", got, want)
	}
	rt.tun.SetCoordinatorSettleGrace(120)
	if got := rt.tun.CoordinatorSettleGrace(); got != 2*time.Minute {
		t.Errorf("configured grace = %v, want 2m", got)
	}
	rt.tun.SetCoordinatorSettleGrace(0)
	if got, want := rt.tun.CoordinatorSettleGrace(), time.Duration(DefaultCoordinatorSettleGraceSec)*time.Second; got != want {
		t.Errorf("0 should mean default, got %v", got)
	}
}

// TestDeepSpawnLeavesHeadroomForShallowWork verifies the depth-aware slot
// reservation: a deep branch may fill most of the global background-turn pool but
// must leave a quarter of it, so shallow work — the level a user or a flow is
// actually waiting on — never starves behind its own descendants.
func TestDeepSpawnLeavesHeadroomForShallowWork(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.tun.SetSpawnLimits(8, 0) // pool of 8 → deep spawns capped at 6
	defer func() {
		// Release everything this test reserved so a later test sees a clean pool.
		for rt.spawnActive.Load() > 0 {
			rt.releaseSpawnSlot()
		}
	}()

	for i := 0; i < 6; i++ {
		if !rt.acquireSpawnSlotAtDepth(3) {
			t.Fatalf("deep spawn %d should fit under the reservation", i+1)
		}
	}
	if rt.acquireSpawnSlotAtDepth(3) {
		t.Fatal("a deep spawn must not take the slots reserved for shallow work")
	}
	// The reserved quarter is still there for the top of the tree.
	if !rt.acquireSpawnSlotAtDepth(1) {
		t.Fatal("a shallow spawn must still get a slot when deep work saturated its share")
	}
	if !rt.acquireSpawnSlot() {
		t.Fatal("a top-level spawn must still get a slot")
	}
	// ...but the global cap still holds for everyone.
	if rt.acquireSpawnSlot() {
		t.Fatal("the global SpawnMaxConcurrent cap must still apply")
	}
}

// TestLegacyWorkerRootsAtItsParent verifies the root fallback for workers created
// before the root was stamped: every pre-rework tree was one level deep, so the
// parent IS the root — without this the shared scratchpad path would move for
// existing sessions.
func TestLegacyWorkerRootsAtItsParent(t *testing.T) {
	legacy := db.Session{ID: "W1", Role: db.SessionRoleWorker, CoordinatorSessionID: "C1"}
	if got := legacy.RootCoordinator(); got != "C1" {
		t.Errorf("legacy worker root = %q, want its coordinator C1", got)
	}
}
