package turnqueue

import (
	"context"
	"sync"
	"testing"
	"time"
)

func newTestQueue() *Queue {
	n := int64(0)
	return New(func() int64 { n++; return n })
}

// waitForWaiting blocks until the session has exactly n queued turns.
func waitForWaiting(t *testing.T, q *Queue, sessionID string, n int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if got := q.Waiting(sessionID); got == n {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("waited for %d queued turns, have %d", n, q.Waiting(sessionID))
		case <-time.After(time.Millisecond):
		}
	}
}

// TestFIFOOrder: the slot goes to whoever asked first. The predecessor was a
// sync.Cond broadcast — every blocked turn woke to race, which is how a queued
// user message could lose repeatedly to a coordinator's own auto-turns.
func TestFIFOOrder(t *testing.T) {
	q := newTestQueue()
	hold, _ := q.Acquire(context.Background(), "S", KindCoordinator, "ilk")

	var mu sync.Mutex
	order := []int{}
	var wg sync.WaitGroup
	for i := 1; i <= 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rel, err := q.Acquire(context.Background(), "S", KindUser, "mesaj")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			mu.Lock()
			order = append(order, n)
			mu.Unlock()
			rel()
		}(i)
		// Queue them one at a time so the arrival order under test is deterministic.
		waitForWaiting(t, q, "S", i)
	}

	hold()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	for i, got := range order {
		if got != i+1 {
			t.Fatalf("handed out in order %v, want 1,2,3,4 (FIFO)", order)
		}
	}
}

// TestNoBargingPastWaiters: a newcomer must not take a slot that is momentarily
// free while someone older is still queued.
func TestNoBargingPastWaiters(t *testing.T) {
	q := newTestQueue()
	hold, _ := q.Acquire(context.Background(), "S", KindUser, "a")

	first := make(chan struct{})
	go func() {
		rel, _ := q.Acquire(context.Background(), "S", KindUser, "b")
		close(first)
		time.Sleep(50 * time.Millisecond)
		rel()
	}()
	waitForWaiting(t, q, "S", 1)

	second := make(chan struct{})
	go func() {
		rel, _ := q.Acquire(context.Background(), "S", KindCoordinator, "c")
		close(second)
		rel()
	}()
	waitForWaiting(t, q, "S", 2)

	hold()
	<-first
	select {
	case <-second:
		t.Fatal("a newer turn barged in front of an older waiter")
	case <-time.After(20 * time.Millisecond):
	}
	<-second
}

// TestCancelledWaiterDoesNotWedge: a caller that gives up (client disconnect,
// timeout) must leave the slot usable — including in the race where the baton is
// handed to it in the same instant it abandons the wait.
func TestCancelledWaiterDoesNotWedge(t *testing.T) {
	q := newTestQueue()
	hold, _ := q.Acquire(context.Background(), "S", KindCoordinator, "tur")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		rel, err := q.Acquire(ctx, "S", KindCommand, "/compact")
		rel() // must be a safe no-op on error
		done <- err
	}()
	waitForWaiting(t, q, "S", 1)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a context error when the wait is abandoned")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled acquire never returned")
	}

	hold()
	got := make(chan struct{})
	go func() {
		rel, _ := q.Acquire(context.Background(), "S", KindUser, "mesaj")
		close(got)
		rel()
	}()
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("slot wedged after a waiter gave up")
	}
}

// TestSnapshotReportsRunningAndWaiting: the queue is observable — this is what the
// chat tray renders, so a user can see WHAT their message is waiting behind.
func TestSnapshotReportsRunningAndWaiting(t *testing.T) {
	q := newTestQueue()
	hold, _ := q.Acquire(context.Background(), "S", KindCoordinator, "worker bildirimi")
	go func() {
		rel, _ := q.Acquire(context.Background(), "S", KindUser, "kullanıcı mesajı")
		rel()
	}()
	waitForWaiting(t, q, "S", 1)

	snap := q.Snapshot("S")
	if snap.Running == nil || snap.Running.Kind != KindCoordinator {
		t.Fatalf("running entry = %+v, want the coordinator turn", snap.Running)
	}
	if len(snap.Waiting) != 1 || snap.Waiting[0].Kind != KindUser {
		t.Fatalf("waiting = %+v, want one user turn", snap.Waiting)
	}
	hold()
}

// TestForgetKeepsLiveState: dropping a session's slot while it is in use would
// strand its waiters, so Forget must refuse until it is idle.
func TestForgetKeepsLiveState(t *testing.T) {
	q := newTestQueue()
	hold, _ := q.Acquire(context.Background(), "S", KindWorker, "görev")
	q.Forget("S")
	if !q.Busy("S") {
		t.Fatal("Forget dropped a slot that was still held")
	}
	hold()
	q.Forget("S")
	if q.Busy("S") {
		t.Fatal("an idle slot should be forgettable")
	}
}
