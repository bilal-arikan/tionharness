package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeferredSubCoordinatorSendsNothingUpward is the TSK766 regression for change
// A. A sub-coordinator whose first turn only fanned work out used to push a
// <task-progress status="delegating"> note into its parent: the parent burned a
// full turn on a non-result and, having nothing to do, narrated a delegation it
// never performed. Nothing may go up now — and because nothing goes up, the
// parent's LIVE worker view is the only carrier of the state, so it must read the
// node as delegating rather than finished.
func TestDeferredSubCoordinatorSendsNothingUpward(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	root := newTreeNode(t, rt, "root-defer", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid-defer", root.ID, root.ID, 1, true)
	newTreeNode(t, rt, "leaf-defer", mid.ID, root.ID, 2, false)

	rt.coordSlotFor(mid.ID).workers.Add(1) // mid's own branch is live
	if !rt.deferWorkerReport(ctx, mid, turnStatusCompleted) {
		t.Fatal("a sub-coordinator with a live branch must withhold its completion")
	}

	msgs, err := rt.db.ListMessages(ctx, root.ID)
	if err != nil {
		t.Fatalf("list root messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("the parent must not be woken while a sub-coordinator delegates, got %d message(s): %+v", len(msgs), msgs)
	}

	// Its own workers now finish, but it still owes a report: the persisted flag
	// alone has to keep it out of the "finished" bucket.
	rt.coordSlotFor(mid.ID).workers.Store(0)

	ws, err := rt.ListWorkers(ctx, root.ID)
	if err != nil {
		t.Fatalf("list root workers: %v", err)
	}
	var found bool
	for _, w := range ws {
		if w.SessionID != mid.ID {
			continue
		}
		found = true
		if !w.Running || !w.Delegating {
			t.Errorf("a node owing a report is not idle: running=%v delegating=%v", w.Running, w.Delegating)
		}
	}
	if !found {
		t.Fatalf("mid missing from the root's worker list: %+v", ws)
	}
	if list := formatWorkerList(ws); !strings.Contains(list, "delegating") {
		t.Errorf("list_workers must spell the state out:\n%s", list)
	}

	sub, err := rt.ListSubtreeWorkers(ctx, root.ID)
	if err != nil {
		t.Fatalf("list root subtree: %v", err)
	}
	tree := formatWorkerTree(sub)
	if !strings.Contains(tree, "delegating") {
		t.Errorf("the subtree listing must spell the state out:\n%s", tree)
	}
	// mid is delegating (counted as running), the leaf below it has finished.
	if !strings.Contains(tree, "1 running, 1 finished") {
		t.Errorf("a delegating node counts as running, not finished:\n%s", tree)
	}
}

// TestReportToCoordinatorDeliversAtEndOfTurn is the TSK766 regression for change
// B. report_to_coordinator used to notify the parent from inside the tool call —
// i.e. while the reporting node was still mid-turn, before its own reply was
// persisted. The note must now be stashed on the running turn and flushed from the
// worker's terminal path, and two calls in one turn must send only the last.
func TestReportToCoordinatorDeliversAtEndOfTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	root := newTreeNode(t, rt, "root-flush", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid-flush", root.ID, root.ID, 1, true)

	rt.setOwesReport(ctx, mid.ID, true)
	upward := &pendingUpwardReport{}
	turnCtx := withPendingUpwardReport(ctx, upward)

	if err := rt.ReportToCoordinator(turnCtx, mid.ID, turnStatusCompleted, "first attempt"); err != nil {
		t.Fatalf("report: %v", err)
	}
	msgs, err := rt.db.ListMessages(ctx, root.ID)
	if err != nil {
		t.Fatalf("list root messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("the report must not reach the parent mid-turn, got %d message(s)", len(msgs))
	}
	// The claim still happens at call time, so the settle backstop cannot race in
	// with a duplicate "incomplete" while the note waits for the turn to end.
	if got, _ := rt.db.GetSession(ctx, mid.ID); got.CoordinatorReportPending {
		t.Error("the pending-report flag must be claimed when the tool is called")
	}

	// Second call in the same turn: the agent corrected itself, only this one ships.
	if err := rt.ReportToCoordinator(turnCtx, mid.ID, turnStatusCompleted, "corrected synthesis"); err != nil {
		t.Fatalf("second report: %v", err)
	}

	rt.flushUpwardReport(upward, mid.ID)
	rt.flushUpwardReport(upward, mid.ID) // idempotent: the terminal path may flush twice

	msgs, err = rt.db.ListMessages(ctx, root.ID)
	if err != nil {
		t.Fatalf("list root messages: %v", err)
	}
	var notes []string
	for _, m := range msgs {
		if strings.Contains(m.Text, "<task-notification>") {
			notes = append(notes, m.Text)
		}
	}
	if len(notes) != 1 {
		t.Fatalf("exactly one report must reach the parent, got %d:\n%s", len(notes), strings.Join(notes, "\n---\n"))
	}
	if !strings.Contains(notes[0], "corrected synthesis") {
		t.Errorf("the last report of the turn must win:\n%s", notes[0])
	}
	if strings.Contains(notes[0], "first attempt") {
		t.Errorf("the superseded report must not be sent:\n%s", notes[0])
	}
	drainSpawns(t, rt)
}

// TestUpwardReportFoldsIntoTerminalNote: a node whose turn IS its result (a leaf
// worker) must not wake the parent twice. The stashed report_to_coordinator is
// consumed by the fold, its weaker status wins over a clean turn, and the flush
// that follows the terminal note sends nothing.
func TestUpwardReportFoldsIntoTerminalNote(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	root := newTreeNode(t, rt, "root-fold", "", "", 0, true)
	leaf := newTreeNode(t, rt, "leaf-fold", root.ID, root.ID, 1, false)

	upward := &pendingUpwardReport{}
	turnCtx := withPendingUpwardReport(ctx, upward)
	if err := rt.ReportToCoordinator(turnCtx, leaf.ID, "incomplete", "ran out of budget"); err != nil {
		t.Fatalf("report: %v", err)
	}
	if got := upward.foldIntoTerminal(turnStatusCompleted); got != "incomplete" {
		t.Fatalf("fold status = %q, want the agent's weaker self-assessment", got)
	}
	rt.flushUpwardReport(upward, leaf.ID)
	msgs, err := rt.db.ListMessages(ctx, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("the folded report must not be delivered on its own, got %d message(s)", len(msgs))
	}
	// No stash → the turn status stands; a reported "completed" never upgrades a
	// failed turn.
	if got := (&pendingUpwardReport{}).foldIntoTerminal(turnStatusCompleted); got != turnStatusCompleted {
		t.Fatalf("empty fold = %q", got)
	}
	up2 := &pendingUpwardReport{}
	up2.stash(root.ID, "n", turnStatusCompleted)
	if got := up2.foldIntoTerminal("failed"); got != "failed" {
		t.Fatalf("fold must not upgrade a failed turn, got %q", got)
	}
	drainSpawns(t, rt)
}

// TestReportToCoordinatorSendsImmediatelyOutsideAWorkerTurn keeps the fallback
// honest: a call with no turn stash (a chat turn opened directly on the worker
// session, or a direct runtime call) has no terminal path to flush it, so it must
// still be delivered on the spot rather than silently dropped.
func TestReportToCoordinatorSendsImmediatelyOutsideAWorkerTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	root := newTreeNode(t, rt, "root-direct", "", "", 0, true)
	mid := newTreeNode(t, rt, "mid-direct", root.ID, root.ID, 1, true)

	if err := rt.ReportToCoordinator(ctx, mid.ID, turnStatusCompleted, "direct synthesis"); err != nil {
		t.Fatalf("report: %v", err)
	}
	msgs, err := rt.db.ListMessages(ctx, root.ID)
	if err != nil {
		t.Fatalf("list root messages: %v", err)
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m.Text, "direct synthesis") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a report with no turn to attach to must be sent immediately, got %d message(s)", len(msgs))
	}
	drainSpawns(t, rt)
}
