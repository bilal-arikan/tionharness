package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// coordination_race_test.go covers the concurrency and crash-boundary cases the
// coordinator tree introduces. They are grouped here because each one is a
// check-then-act that only misbehaves under real overlap — exactly the shape a
// coordinator produces on purpose when it fans out in a single turn.

// TestReportClaimIsExclusive is the guard for a doubly-reported task. Two settle
// backstops can be armed for one session (one per drain exit) and the agent's own
// report_to_coordinator can land at the same moment; without an atomic claim each
// passes its own "does it still owe a report?" check and sends, so the coordinator
// above sees one subtask reported twice with conflicting statuses.
func TestReportClaimIsExclusive(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	root := newTreeNode(t, rt, "root", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid", root.ID, root.ID, 1, true)
	rt.setOwesReport(ctx, mid.ID, true)

	const racers = 16
	var wins int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			won, err := rt.db.ClaimCoordinatorReport(ctx, mid.ID)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			if won {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Fatalf("exactly one caller may own the report, got %d winners", wins)
	}
	got, _ := rt.db.GetSession(ctx, mid.ID)
	if got.CoordinatorReportPending {
		t.Error("the claim must clear the pending flag")
	}
}

// TestSettleBackstopDoesNotDoubleReport drives the same race through the real
// backstop: only one of many concurrent sweeps may put a notification into the
// coordinator's history.
func TestSettleBackstopDoesNotDoubleReport(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	root := newTreeNode(t, rt, "root", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid", root.ID, root.ID, 1, true)
	if _, err := rt.db.AddMessage(ctx, db.Message{
		SessionID: mid.ID, AgentID: mid.AgentID, Role: "assistant", Text: "worker açtım",
	}); err != nil {
		t.Fatalf("seed reply: %v", err)
	}
	rt.setOwesReport(ctx, mid.ID, true)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; rt.settleReportBackstop(ctx, mid.ID) }()
	}
	close(start)
	wg.Wait()

	msgs, err := rt.db.ListMessages(ctx, root.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	n := 0
	for _, m := range msgs {
		if m.Origin == "worker-note" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("coordinator received %d backstop notifications, want exactly 1", n)
	}
}

// TestSubtreeBudgetHoldsUnderConcurrentSpawns is the guard for the budget that
// actually stops exponential fan-out. Counting live workers and then creating one
// is a check-then-act: a coordinator fanning out in ONE turn issues its
// spawn_worker calls concurrently, so without a per-tree lock they could all read
// the same remaining capacity and every one of them creates. Seeding the tree to
// capacity with live workers makes every concurrent spawn a refusal — none may
// slip a new session past the guard.
func TestSubtreeBudgetHoldsUnderConcurrentSpawns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	const budget = 3
	rt.tun.SetCoordinatorLimits(64, 0, 0, budget) // worker cap high: the TREE cap is under test
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	root := newTestCoordinator(t, rt, 0)

	// Fill the tree to capacity with live workers, so the budget is already spent
	// when the concurrent storm hits.
	seeded := make([]string, 0, budget)
	for i := 0; i < budget; i++ {
		w := newTreeNode(t, rt, fmt.Sprintf("seed%d", i), root, root, 1, false)
		rt.trackSession(w.ID, func() {})
		seeded = append(seeded, w.ID)
	}
	defer func() {
		for _, id := range seeded {
			rt.untrackSession(id)
		}
	}()

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < budget*4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _ = rt.SpawnWorker(ctx, root, "W", "task", "", WorkerSpec{})
		}()
	}
	close(start)
	wg.Wait()

	// Not one concurrent spawn may have created a session: the tree was already at
	// capacity with live workers, and the per-tree lock keeps every check honest.
	tree, err := rt.db.ListCoordinatorTree(ctx, root)
	if err != nil {
		t.Fatalf("list tree: %v", err)
	}
	if workers := len(tree) - 1; workers > budget {
		t.Fatalf("tree budget overshot: %d worker sessions in the tree, cap is %d", workers, budget)
	}
}

// TestPendingReportSurvivesProcessRestart simulates the crash boundary: the
// in-memory coordination state (coordSlot) is gone, but the owed report must not
// be, and the boot sweep must find exactly the sessions that still owe one.
func TestPendingReportSurvivesProcessRestart(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	workDir := filepath.Join(t.TempDir(), "workspace")
	ctx := context.Background()

	rt := runtimeOverStore(t, storeDir, workDir)
	root := newTreeNode(t, rt, "root", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid", root.ID, root.ID, 1, true)
	// A leaf that already finished: nothing is running, so the branch is settled.
	newTreeNode(t, rt, "leaf", mid.ID, root.ID, 2, false)
	rt.setOwesReport(ctx, mid.ID, true)

	// "Restart": a second runtime over the SAME store, with empty in-memory slots —
	// the state a process crash leaves behind.
	rt2 := runtimeOverStore(t, storeDir, workDir)
	pending, err := rt2.db.ListPendingCoordinatorReports(ctx)
	if err != nil {
		t.Fatalf("boot scan: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != mid.ID {
		t.Fatalf("boot scan found %+v, want just the mid node", pending)
	}
	// And the boot sweep can still deliver it — the slot being empty must not make
	// the backstop believe the report was already sent.
	rt2.settleReportBackstop(ctx, mid.ID)
	msgs, err := rt2.db.ListMessages(ctx, root.ID)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("coordinator got nothing after restart (err=%v)", err)
	}
	if again, _ := rt2.db.ListPendingCoordinatorReports(ctx); len(again) != 0 {
		t.Errorf("the report should be closed after delivery, still pending: %+v", again)
	}
}

// runtimeOverStore builds a Runtime on an EXPLICIT store directory, so a test can
// open two of them over the same on-disk state and exercise a restart.
// newTestRuntime cannot: it allocates a fresh t.TempDir() store per call, and its
// workDir argument is the sandbox root, not the store.
func runtimeOverStore(t *testing.T, storeDir, workDir string) *Runtime {
	t.Helper()
	database, err := db.Open(storeDir)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return NewRuntime(database, providers.NewRegistry(), NewTunables(), workDir, nil, nil, "", "",
		nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}
