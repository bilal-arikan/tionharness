package agent

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// lastCoordNote returns the text of the most recent user-role note in a coordinator
// session, which is the message the next coordinator turn opens on.
func lastCoordNote(t *testing.T, rt *Runtime, coord string) string {
	t.Helper()
	msgs, err := rt.db.ListMessages(context.Background(), coord)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Text
		}
	}
	t.Fatal("no user note recorded in the coordinator session")
	return ""
}

// countCoordStatusNotes counts how many separate history entries carry the all-idle
// signal. The fold must produce exactly one piggybacked message.
func countCoordStatusNotes(t *testing.T, rt *Runtime, coord string) int {
	t.Helper()
	msgs, err := rt.db.ListMessages(context.Background(), coord)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	n := 0
	for _, m := range msgs {
		if strings.Contains(m.Text, "<coordination-status>") {
			n++
		}
	}
	return n
}

// TestLastWorkerNotificationCarriesIdleStatus is the core contract: when the LAST
// worker's result lands, the all-idle signal must ride ON that <task-notification>
// and the coordinator must get exactly ONE turn — not a second, otherwise-identical
// turn whose only new information is "the workers you just heard from are done".
func TestLastWorkerNotificationCarriesIdleStatus(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	defer drainSpawns(t, rt)
	coord := newTestCoordinator(t, rt, 0)

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	slot := rt.coordSlotFor(coord)
	slot.markHadWorkers()
	slot.workers.Add(1) // the one and only worker, about to finish

	// The worker finishes exactly as runWorker does: release, then notify with the
	// zero-crossing observation that release returned.
	rt.notifyCoordinator(coord, "<task-notification>SES9 completed</task-notification>", releaseOnce(slot)(), []TurnStep{{
		Kind:    StepDiff,
		Tool:    "Edit",
		Path:    "main.go",
		Patch:   "-old\n+new",
		Added:   1,
		Removed: 1,
	}}, "")

	waitTurns(t, &mu, &turns, 1, "the single folded turn")

	// The decisive assertion: no SECOND turn. Before the fold this was 2.
	time.Sleep(250 * time.Millisecond)
	mu.Lock()
	got := turns
	mu.Unlock()
	if got != 1 {
		t.Fatalf("last-worker notification must not cost a second turn; got %d turns", got)
	}

	note := lastCoordNote(t, rt, coord)
	if !strings.Contains(note, "<task-notification>") {
		t.Errorf("the folded note must still be the worker result:\n%s", note)
	}
	if !strings.Contains(note, "<coordination-status>") {
		t.Errorf("the last worker's notification must carry the all-idle signal:\n%s", note)
	}
	msgs, err := rt.db.ListMessages(context.Background(), coord)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) == 0 || !strings.Contains(msgs[0].Steps, `"kind":"diff"`) ||
		!strings.Contains(msgs[0].Steps, `"path":"main.go"`) {
		t.Fatalf("worker notification did not preserve diff steps: %#v", msgs)
	}
	if n := countCoordStatusNotes(t, rt, coord); n != 1 {
		t.Fatalf("the all-idle signal must appear exactly once, got %d copies", n)
	}
}

// TestIdleStatusNotFoldedWhileWorkersRemain: a worker finishing while OTHERS are
// still running is not an all-idle transition. Folding the note there would tell the
// coordinator everyone is done while its fleet is still working — the precise lie
// that would make it conclude early.
func TestIdleStatusNotFoldedWhileWorkersRemain(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	defer drainSpawns(t, rt)
	coord := newTestCoordinator(t, rt, 0)
	rt.coordRunFn = func(string) {}

	slot := rt.coordSlotFor(coord)
	slot.markHadWorkers()
	slot.workers.Add(2) // two workers running

	// Only the first finishes: its release does not reach zero, so no fold.
	rt.notifyCoordinator(coord, "<task-notification>SES1 completed</task-notification>", releaseOnce(slot)(), nil, "")

	note := lastCoordNote(t, rt, coord)
	if strings.Contains(note, "<coordination-status>") {
		t.Fatalf("all-idle signal folded while a worker is still running:\n%s", note)
	}
}

// TestIdleStatusFoldedOnceAcrossConcurrentFinishes: workers finishing concurrently
// must yield exactly ONE all-idle signal. The counter read and the ackedIdle claim
// happen under one lock precisely so two racing finishers cannot both believe they
// were last (duplicate note) or both defer to the other (lost signal).
func TestIdleStatusFoldedOnceAcrossConcurrentFinishes(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	defer drainSpawns(t, rt)
	coord := newTestCoordinator(t, rt, 0)
	rt.coordRunFn = func(string) {}

	const n = 8
	slot := rt.coordSlotFor(coord)
	slot.markHadWorkers()
	slot.workers.Add(n)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all finishers at once to maximise the overlap
			rt.notifyCoordinator(coord, "<task-notification>done</task-notification>", releaseOnce(slot)(), nil, "")
		}()
	}
	close(start)
	wg.Wait()

	if got := countCoordStatusNotes(t, rt, coord); got != 1 {
		t.Fatalf("concurrent finishes must yield exactly one all-idle signal, got %d", got)
	}

	// Then let the drain loop run to completion. This is the assertion that matters:
	// the earlier bug folded correctly and THEN had the sweep undo it — the siblings'
	// notifications cleared ackedIdle and the standalone note was injected anyway,
	// ending at 3 copies plus the extra turn the fold exists to remove.
	time.Sleep(400 * time.Millisecond)
	if got := countCoordStatusNotes(t, rt, coord); got != 1 {
		t.Fatalf("the drain sweep must not re-inject the all-idle note; got %d copies", got)
	}
}

// TestSecondWaveGetsItsOwnIdleSignal: after a fold, spawning another worker opens a
// NEW all-idle transition that must be reported again. Suppressing the sweep at
// workers==0 is only safe because a spawn/continuation re-arms it — without that
// re-arm the second wave's completion would be silent, stranding the coordinator.
func TestSecondWaveGetsItsOwnIdleSignal(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	defer drainSpawns(t, rt)
	coord := newTestCoordinator(t, rt, 0)
	rt.coordRunFn = func(string) {}

	slot := rt.coordSlotFor(coord)
	slot.markHadWorkers()

	// First wave: one worker finishes and folds the signal.
	slot.workers.Add(1)
	rt.notifyCoordinator(coord, "<task-notification>wave1</task-notification>", releaseOnce(slot)(), nil, "")
	if got := countCoordStatusNotes(t, rt, coord); got != 1 {
		t.Fatalf("first wave must fold exactly one signal, got %d", got)
	}

	// Second wave: a fresh worker re-arms the sweep the way the spawn path does.
	slot.workers.Add(1)
	slot.mu.Lock()
	slot.ackedIdle = false
	slot.idleFolded = false
	slot.mu.Unlock()

	rt.notifyCoordinator(coord, "<task-notification>wave2</task-notification>", releaseOnce(slot)(), nil, "")

	note := lastCoordNote(t, rt, coord)
	if !strings.Contains(note, "wave2") || !strings.Contains(note, "<coordination-status>") {
		t.Fatalf("the second wave's last worker must carry its own all-idle signal:\n%s", note)
	}
	if got := countCoordStatusNotes(t, rt, coord); got != 2 {
		t.Fatalf("each wave gets exactly one signal; got %d total", got)
	}
}

// TestIdleReconcileNeverSendsStandaloneNotification pins the no-second-turn
// contract. An unobserved zero-worker state must not manufacture a fleet-finished
// message; only a real last-worker result may carry that signal.
func TestIdleReconcileNeverSendsStandaloneNotification(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	defer drainSpawns(t, rt)
	coord := newTestCoordinator(t, rt, 0)

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	slot := rt.coordSlotFor(coord)
	slot.markHadWorkers() // workers already at 0: the transition nobody notified about

	rt.enqueueCoordinatorTurn(coord)
	waitTurns(t, &mu, &turns, 1, "single processing turn")
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	gotTurns := turns
	mu.Unlock()
	if gotTurns != 1 {
		t.Fatalf("idle reconcile must not create a standalone turn; got %d", gotTurns)
	}

	if got := countCoordStatusNotes(t, rt, coord); got != 0 {
		t.Fatalf("idle reconcile must not inject a standalone note, got %d", got)
	}
}

// TestReleaseOnceIsIdempotent guards the counter runWorker now releases explicitly
// before notifying and again in its defer. A double decrement would under-count the
// fleet, making a still-running worker look idle — and fold the all-idle note into a
// notification while the coordinator's other workers are mid-flight.
func TestReleaseOnceIsIdempotent(t *testing.T) {
	slot := &coordSlot{}
	slot.workers.Add(2)

	release := releaseOnce(slot)
	if release() {
		t.Fatal("a release that leaves a worker running must not report last")
	}
	// Repeat calls must neither decrement again nor change the verdict.
	release()
	release()

	if got := slot.workers.Load(); got != 1 {
		t.Fatalf("releaseOnce must decrement exactly once; workers=%d, want 1", got)
	}

	// The remaining worker's release is the zero-crossing, and it stays true on repeat.
	last := releaseOnce(slot)
	if !last() {
		t.Fatal("the release that reaches zero must report last")
	}
	if !last() {
		t.Fatal("the last-worker verdict must be stable across repeat calls")
	}
	if got := slot.workers.Load(); got != 0 {
		t.Fatalf("workers=%d, want 0", got)
	}

	if releaseOnce(nil)() {
		t.Fatal("a nil slot must never report a zero-crossing")
	}
}

// TestFoldReleasedWhenNotePersistFails: the fold is a claim that a specific message
// carries the signal. If that write fails the claim must be given back, or the
// coordinator loses the all-idle signal entirely — the stall this whole mechanism
// exists to prevent.
func TestFoldReleasedWhenNotePersistFails(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.coordRunFn = func(string) {}

	// An unknown session id makes the note write fail.
	const missing = "SES-does-not-exist"
	slot := rt.coordSlotFor(missing)
	slot.markHadWorkers()
	slot.workers.Add(1)

	rt.notifyCoordinator(missing, "<task-notification>orphan</task-notification>", releaseOnce(slot)(), nil, "")

	slot.mu.Lock()
	acked := slot.ackedIdle
	slot.mu.Unlock()
	if acked {
		t.Fatal("a failed note write must not leave the all-idle signal claimed")
	}
}
