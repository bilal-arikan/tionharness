package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func waitForCondition(t *testing.T, timeout time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for {
		if ok() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", what)
		case <-tick.C:
		}
	}
}

func TestCoordinatorWorkerBatchWindowDefault(t *testing.T) {
	if got := NewTunables().CoordinatorWorkerBatchWindow(); got != 5*time.Second {
		t.Fatalf("CoordinatorWorkerBatchWindow = %s, want 5s", got)
	}
}

func TestWorkerNotesBatchFromFirstArrivalWithoutExtendingDeadline(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(120 * time.Millisecond)
	coord := newTestCoordinator(t, rt, 0)

	started := make(chan struct{}, 2)
	rt.coordRunFn = func(string) { started <- struct{}{} }
	rt.enqueueCoordinatorWorkerTurn(coord, false)
	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	firstDeadline := slot.workerDeadline
	slot.mu.Unlock()

	select {
	case <-started:
		t.Fatal("worker turn started before batching window elapsed")
	case <-time.After(35 * time.Millisecond):
	}

	rt.enqueueCoordinatorWorkerTurn(coord, false)
	slot.mu.Lock()
	secondDeadline := slot.workerDeadline
	slot.mu.Unlock()
	if !secondDeadline.Equal(firstDeadline) {
		t.Fatalf("second worker note extended deadline: first=%s second=%s", firstDeadline, secondDeadline)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("batched worker notes never started coordinator turn")
	}
	waitForCondition(t, time.Second, "batch drain to become idle", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return !slot.driving
	})
	select {
	case <-started:
		t.Fatal("two notes in first window produced more than one turn")
	case <-time.After(80 * time.Millisecond):
	}
}

func TestWorkerNoteMidTurnWaitsOnlyRemainingWindow(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(200 * time.Millisecond)
	coord := newTestCoordinator(t, rt, 0)

	started := make(chan time.Time, 2)
	releaseFirst := make(chan struct{})
	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) {
		mu.Lock()
		turns++
		n := turns
		mu.Unlock()
		started <- time.Now()
		if n == 1 {
			<-releaseFirst
		}
	}

	rt.enqueueCoordinatorTurn(coord)
	<-started
	rt.enqueueCoordinatorWorkerTurn(coord, false)
	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	deadline := slot.workerDeadline
	slot.mu.Unlock()
	time.Sleep(80 * time.Millisecond)
	close(releaseFirst)

	var second time.Time
	select {
	case second = <-started:
	case <-time.After(time.Second):
		t.Fatal("mid-turn worker note did not produce follow-up")
	}
	if second.Before(deadline.Add(-15 * time.Millisecond)) {
		t.Fatalf("follow-up started before remaining window elapsed: start=%s deadline=%s", second, deadline)
	}
	if second.After(deadline.Add(60 * time.Millisecond)) {
		t.Fatalf("follow-up restarted the full window: start=%s deadline=%s", second, deadline)
	}
}

func TestWorkerNoteDeadlinePastAtTurnEndRunsImmediately(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(40 * time.Millisecond)
	coord := newTestCoordinator(t, rt, 0)

	started := make(chan time.Time, 2)
	releaseFirst := make(chan struct{})
	var turns int
	var mu sync.Mutex
	rt.coordRunFn = func(string) {
		mu.Lock()
		turns++
		n := turns
		mu.Unlock()
		started <- time.Now()
		if n == 1 {
			<-releaseFirst
		}
	}
	rt.enqueueCoordinatorTurn(coord)
	<-started
	rt.enqueueCoordinatorWorkerTurn(coord, false)
	time.Sleep(70 * time.Millisecond)
	releasedAt := time.Now()
	close(releaseFirst)
	select {
	case second := <-started:
		if second.Sub(releasedAt) > 100*time.Millisecond {
			t.Fatalf("past-deadline follow-up was not immediate: delay=%s", second.Sub(releasedAt))
		}
	case <-time.After(time.Second):
		t.Fatal("past-deadline follow-up never started")
	}
}

func TestGenericWakeInterruptsWorkerBatchWithoutStaleFollowup(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(2 * time.Second)
	coord := newTestCoordinator(t, rt, 0)

	started := make(chan struct{}, 2)
	rt.coordRunFn = func(string) { started <- struct{}{} }
	rt.enqueueCoordinatorWorkerTurn(coord, false)
	rt.enqueueCoordinatorTurn(coord)
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("generic wake did not interrupt worker batch wait")
	}
	slot := rt.coordSlotFor(coord)
	waitForCondition(t, time.Second, "generic drain to become idle", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return !slot.driving
	})
	slot.mu.Lock()
	stale := slot.workerPending || !slot.workerDeadline.IsZero()
	slot.mu.Unlock()
	if stale {
		t.Fatal("generic turn left stale worker batch state")
	}
	select {
	case <-started:
		t.Fatal("generic turn left a redundant worker follow-up")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWorkerNotePersistenceBarrierSerializesGenericWake(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(2 * time.Second)
	coord := newTestCoordinator(t, rt, 0)

	persisted := make(chan struct{})
	releasePersist := make(chan struct{})
	rt.coordAfterWorkerNotePersist = func(string) {
		close(persisted)
		<-releasePersist
	}
	started := make(chan struct{}, 2)
	rt.coordRunFn = func(string) { started <- struct{}{} }
	notifyDone := make(chan error, 1)
	go func() {
		notifyDone <- rt.notifyCoordinator(coord, "<task-notification>durable</task-notification>", false, nil, "")
	}()
	<-persisted

	rt.enqueueCoordinatorTurn(coord)
	select {
	case <-started:
		t.Fatal("generic drain crossed worker-note persistence barrier")
	case <-time.After(50 * time.Millisecond):
	}
	close(releasePersist)
	if err := <-notifyDone; err != nil {
		t.Fatalf("notify coordinator: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("serialized generic wake did not run")
	}
	slot := rt.coordSlotFor(coord)
	waitForCondition(t, time.Second, "serialized drain to become idle", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return !slot.driving
	})
	if note := lastCoordNote(t, rt, coord); note != "<task-notification>durable</task-notification>" {
		t.Fatalf("durable worker note missing: %q", note)
	}
	slot.mu.Lock()
	stale := slot.workerPending || !slot.workerDeadline.IsZero()
	slot.mu.Unlock()
	if stale {
		t.Fatal("generic wake left redundant worker deadline")
	}
	select {
	case <-started:
		t.Fatal("durable note and generic wake produced follow-up turn")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestArchiveAfterFIFOTurnAdmissionRejectsFinalGate(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(2 * time.Second)
	coord := newTestCoordinator(t, rt, 0)
	started := make(chan struct{}, 1)
	rt.coordRunFn = func(string) { started <- struct{}{} }

	if err := rt.notifyCoordinator(coord, "<task-notification>durable</task-notification>", false, nil, ""); err != nil {
		t.Fatalf("notify coordinator: %v", err)
	}
	slot := rt.coordSlotFor(coord)
	<-slot.admission
	releasedAdmission := false
	defer func() {
		if !releasedAdmission {
			slot.admission <- struct{}{}
		}
	}()
	rt.enqueueCoordinatorTurn(coord)
	waitForCondition(t, time.Second, "coordinator FIFO turn admission", func() bool {
		return rt.TurnQueue().Snapshot(coord).Running != nil
	})
	if err := rt.db.SetSessionState(context.Background(), coord, "archived"); err != nil {
		t.Fatalf("archive coordinator: %v", err)
	}
	slot.admission <- struct{}{}
	releasedAdmission = true
	waitForCondition(t, time.Second, "archive rejection cleanup", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return !slot.driving
	})
	select {
	case <-started:
		t.Fatal("archived coordinator crossed final turn-start gate")
	default:
	}
	if note := lastCoordNote(t, rt, coord); note != "<task-notification>durable</task-notification>" {
		t.Fatalf("archive rejection lost durable note: %q", note)
	}
	slot.mu.Lock()
	armed := slot.pending || slot.workerPending || !slot.workerDeadline.IsZero()
	slot.mu.Unlock()
	if armed {
		t.Fatal("archive final-gate rejection left transient state")
	}
}

func TestArchiveAfterFinalGateLetsActiveTurnFinishOnce(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(-1)
	coord := newTestCoordinator(t, rt, 0)
	started := make(chan struct{}, 2)
	releaseRun := make(chan struct{})
	rt.coordRunFn = func(string) {
		started <- struct{}{}
		<-releaseRun
	}

	if err := rt.notifyCoordinator(coord, "<task-notification>first</task-notification>", false, nil, ""); err != nil {
		t.Fatalf("first notify: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first turn did not cross final gate")
	}
	if err := rt.notifyCoordinator(coord, "<task-notification>second</task-notification>", false, nil, ""); err != nil {
		t.Fatalf("second notify: %v", err)
	}
	if err := rt.db.SetSessionState(context.Background(), coord, "archived"); err != nil {
		t.Fatalf("archive coordinator: %v", err)
	}
	close(releaseRun)
	slot := rt.coordSlotFor(coord)
	waitForCondition(t, time.Second, "post-turn archive cleanup", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return !slot.driving
	})
	select {
	case <-started:
		t.Fatal("archive after final gate allowed a follow-up turn")
	case <-time.After(100 * time.Millisecond):
	}
	slot.mu.Lock()
	turns := slot.turns
	slot.mu.Unlock()
	if turns != 1 {
		t.Fatalf("accepted active turn count = %d, want 1", turns)
	}
}

func TestWorkerNotePersistenceFailurePreservesGenericWake(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)
	started := make(chan struct{}, 1)
	rt.coordRunFn = func(string) { started <- struct{}{} }

	releaseUser := rt.BeginSessionUserTurn(coord)
	rt.enqueueCoordinatorTurn(coord)
	waitForCondition(t, time.Second, "generic wake to queue", func() bool {
		return rt.TurnQueue().Waiting(coord) == 1
	})
	if err := os.RemoveAll(filepath.Join(rt.db.Root(), "sessions", coord)); err != nil {
		t.Fatalf("remove coordinator transcript directory: %v", err)
	}
	if err := rt.notifyCoordinator(coord, "<task-notification>must fail</task-notification>", false, nil, ""); err == nil {
		t.Fatal("worker note persistence unexpectedly succeeded")
	}
	releaseUser()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker-note persistence failure lost generic wake")
	}
	slot := rt.coordSlotFor(coord)
	waitForCondition(t, time.Second, "generic drain after persistence failure", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return !slot.driving
	})
	slot.mu.Lock()
	turns := slot.turns
	slot.mu.Unlock()
	if turns != 1 {
		t.Fatalf("generic wake ran %d turns after persistence failure, want 1", turns)
	}
}

func TestWorkerBatchArchivePreservesDurableNoteAndRunsNoTurn(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(80 * time.Millisecond)
	coord := newTestCoordinator(t, rt, 0)
	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) { mu.Lock(); turns++; mu.Unlock() }

	rt.NotifyCoordinator(coord, "<task-notification>durable</task-notification>")
	if err := rt.db.SetSessionState(context.Background(), coord, "archived"); err != nil {
		t.Fatalf("archive coordinator: %v", err)
	}
	time.Sleep(180 * time.Millisecond)
	mu.Lock()
	gotTurns := turns
	mu.Unlock()
	if gotTurns != 0 {
		t.Fatalf("archived batch ran %d turns, want 0", gotTurns)
	}
	if note := lastCoordNote(t, rt, coord); note != "<task-notification>durable</task-notification>" {
		t.Fatalf("durable worker note missing after archive: %q", note)
	}
	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	armed := slot.driving || slot.pending || slot.workerPending || !slot.workerDeadline.IsZero()
	slot.mu.Unlock()
	if armed {
		t.Fatal("archive rejection left transient batch state armed")
	}
	if err := rt.db.SetSessionState(context.Background(), coord, "active"); err != nil {
		t.Fatalf("unarchive coordinator: %v", err)
	}
	rt.enqueueCoordinatorTurn(coord)
	waitForCondition(t, time.Second, "unarchived generic turn", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return turns == 1
	})
	waitForCondition(t, time.Second, "unarchived drain to become idle", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return !slot.driving
	})
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	gotTurns = turns
	mu.Unlock()
	if gotTurns != 1 {
		t.Fatalf("unarchive generic resume produced %d turns, want exactly 1", gotTurns)
	}
}

func TestWorkerBatchStallHaltThenResumeHasNoStaleBatch(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(2 * time.Second)
	coord := newTestCoordinator(t, rt, 0)
	started := make(chan struct{}, 2)
	rt.coordRunFn = func(string) { started <- struct{}{} }
	rt.NotifyCoordinator(coord, "<task-notification>halt me</task-notification>")

	slot := rt.coordSlotFor(coord)
	agent := db.Agent{ID: "A", Name: "Coord"}
	rt.escalateCoordinatorStallHalt(coord, agent.ID, agent, slot, "test")
	waitForCondition(t, time.Second, "halted batch drain to stop", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return !slot.driving
	})
	select {
	case <-started:
		t.Fatal("stall-halted batch ran a turn")
	default:
	}
	if note := lastCoordNote(t, rt, coord); note != "<task-notification>halt me</task-notification>" {
		t.Fatalf("stall halt did not preserve durable worker note: %q", note)
	}
	if err := rt.ResumeCoordinatorFromStall(context.Background(), coord); err != nil {
		t.Fatalf("resume coordinator: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("resume did not run immediate generic turn")
	}
	waitForCondition(t, time.Second, "resumed drain to become idle", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return !slot.driving
	})
	select {
	case <-started:
		t.Fatal("resume inherited stale worker batch follow-up")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestConcurrentWorkerNotesStaySerializedAndBounded(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(35 * time.Millisecond)
	coord := newTestCoordinator(t, rt, 0)

	var mu sync.Mutex
	concurrent, maxConcurrent, turns := 0, 0, 0
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	rt.coordRunFn = func(string) {
		mu.Lock()
		concurrent++
		turns++
		n := turns
		if concurrent > maxConcurrent {
			maxConcurrent = concurrent
		}
		mu.Unlock()
		if n == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		mu.Lock()
		concurrent--
		mu.Unlock()
	}
	rt.enqueueCoordinatorWorkerTurn(coord, false)
	<-firstStarted
	var notes sync.WaitGroup
	for i := 0; i < 100; i++ {
		notes.Add(1)
		go func() {
			defer notes.Done()
			rt.enqueueCoordinatorWorkerTurn(coord, false)
		}()
	}
	notes.Wait()
	time.Sleep(50 * time.Millisecond)
	close(releaseFirst)
	slot := rt.coordSlotFor(coord)
	waitForCondition(t, time.Second, "concurrent worker drain to settle", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return !slot.driving
	})
	mu.Lock()
	defer mu.Unlock()
	if maxConcurrent != 1 {
		t.Fatalf("coordinator turns overlapped: maxConcurrent=%d", maxConcurrent)
	}
	if turns != 2 {
		t.Fatalf("continuous worker notes produced %d turns, want 2 coalesced turns", turns)
	}
}

func TestContinuousWorkerWakesCannotStarvePastDeadlineAdmission(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(40 * time.Millisecond)
	coord := newTestCoordinator(t, rt, 0)

	started := make(chan time.Time, 1)
	rt.coordRunFn = func(string) {
		select {
		case started <- time.Now():
		default:
		}
	}
	rt.enqueueCoordinatorWorkerTurn(coord, false)
	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	deadline := slot.workerDeadline
	slot.mu.Unlock()

	stop := make(chan struct{})
	var wakes sync.WaitGroup
	for i := 0; i < 8; i++ {
		wakes.Add(1)
		go func() {
			defer wakes.Done()
			for {
				select {
				case <-stop:
					return
				default:
					rt.enqueueCoordinatorWorkerTurn(coord, false)
				}
			}
		}()
	}

	select {
	case first := <-started:
		close(stop)
		wakes.Wait()
		if first.After(deadline.Add(150 * time.Millisecond)) {
			t.Fatalf("continuous worker wakes starved post-deadline admission: start=%s deadline=%s", first, deadline)
		}
	case <-time.After(time.Until(deadline) + 300*time.Millisecond):
		close(stop)
		wakes.Wait()
		t.Fatal("continuous worker wakes starved coordinator turn past batching deadline")
	}
}

func TestRuntimeCloseCancelsWorkerBatchWait(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetCoordinatorWorkerBatchWindow(5 * time.Second)
	coord := newTestCoordinator(t, rt, 0)
	rt.coordRunFn = func(string) { t.Error("batch turn ran during shutdown") }
	rt.enqueueCoordinatorWorkerTurn(coord, false)
	slot := rt.coordSlotFor(coord)
	waitForCondition(t, time.Second, "batch drain to start", func() bool {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		return slot.driving
	})

	closed := make(chan struct{})
	go func() {
		rt.CloseMCP()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("CloseMCP did not cancel worker batch wait")
	}
	slot.mu.Lock()
	armed := slot.driving || slot.pending || slot.workerPending || !slot.workerDeadline.IsZero()
	slot.mu.Unlock()
	if armed {
		t.Fatal("runtime close left coordinator batch state armed")
	}
	if _, err := rt.db.ListMessages(context.Background(), coord); err != nil {
		t.Fatalf("coordinator drain touched closed/unavailable DB after cleanup: %v", err)
	}
}

func TestRuntimeCloseCancelsCoordinatorAdmissionWait(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)
	rt.coordRunFn = func(string) { t.Error("coordinator turn ran during shutdown") }
	slot := rt.coordSlotFor(coord)
	<-slot.admission
	releasedAdmission := false
	defer func() {
		if !releasedAdmission {
			slot.admission <- struct{}{}
		}
	}()
	rt.enqueueCoordinatorTurn(coord)
	waitForCondition(t, time.Second, "drain to wait on coordinator admission", func() bool {
		return rt.TurnQueue().Snapshot(coord).Running != nil
	})

	closed := make(chan struct{})
	go func() {
		rt.CloseMCP()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("CloseMCP did not cancel coordinator admission wait")
	}
	slot.admission <- struct{}{}
	releasedAdmission = true
	slot.mu.Lock()
	armed := slot.driving || slot.pending || slot.workerPending || !slot.workerDeadline.IsZero()
	slot.mu.Unlock()
	if armed {
		t.Fatal("runtime close left admission-wait state armed")
	}
	if rt.sessionTurnBusy(coord) {
		t.Fatal("runtime close left coordinator FIFO turn slot held")
	}
}
