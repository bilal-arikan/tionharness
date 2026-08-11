package agent

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/turnqueue"
)

// waitForTurnWaiters blocks until the session's admission queue has n queued turns.
func waitForTurnWaiters(t *testing.T, rt *Runtime, sessionID string, n int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if got := rt.TurnQueue().Waiting(sessionID); got == n {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("waited for %d queued turns, have %d", n, rt.TurnQueue().Waiting(sessionID))
		case <-time.After(time.Millisecond):
		}
	}
}

// TestCoordinatorYieldsSlotToWaitingTurn is the fairness guard for the bug the
// admission queue exists for (_Docs/58): while a coordinator drains its own
// auto-turns, a user turn waiting for the session must run after the CURRENT turn —
// not after the whole drain. The drain re-queues each iteration, so this is
// structural rather than a special case. The pending notification is not dropped;
// it simply runs after the human.
func TestCoordinatorYieldsSlotToWaitingTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	autoTurns := 0
	inTurn := make(chan struct{}, 1)
	proceed := make(chan struct{})
	rt.coordRunFn = func(string) {
		mu.Lock()
		autoTurns++
		n := autoTurns
		mu.Unlock()
		if n == 1 {
			inTurn <- struct{}{}
			<-proceed
		}
	}

	rt.enqueueCoordinatorTurn("COORD")
	<-inTurn // the first auto turn holds the slot

	// The user's queued message arrives mid-turn and joins the FIFO...
	userRan := make(chan struct{})
	go func() {
		rel := rt.BeginSessionUserTurn("COORD")
		close(userRan)
		rel()
	}()
	waitForTurnWaiters(t, rt, "COORD", 1)

	// ...and a worker notification lands too, which re-arms the drain for another
	// auto-turn. It must NOT overtake the user.
	rt.enqueueCoordinatorTurn("COORD")

	close(proceed)

	select {
	case <-userRan:
	case <-time.After(2 * time.Second):
		t.Fatal("the coordinator drain kept the slot instead of yielding to the waiting user turn")
	}

	// The re-armed notification still gets its turn, after the human.
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := autoTurns
		mu.Unlock()
		if n >= 2 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("the pending notification was dropped when the drain yielded")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestUserTurnBlocksCoordinatorTurn: the reverse direction — a coordinator
// notification that lands while a user turn holds the session waits for it, and
// never runs concurrently.
func TestUserTurnBlocksCoordinatorTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) {
		mu.Lock()
		turns++
		mu.Unlock()
	}

	release := rt.BeginSessionUserTurn("COORD")
	rt.enqueueCoordinatorTurn("COORD")
	waitForTurnWaiters(t, rt, "COORD", 1)

	mu.Lock()
	n := turns
	mu.Unlock()
	if n != 0 {
		t.Fatalf("a coordinator turn ran while a user turn held the session (%d turns)", n)
	}

	release()
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := turns
		mu.Unlock()
		if n >= 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("the coordinator turn never ran after the user turn released")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestAdmissionQueueLabelsWaiters: every entry path names itself, so the chat tray
// can tell the user WHAT their message is queued behind rather than just "busy".
func TestAdmissionQueueLabelsWaiters(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	hold := rt.claimSessionTurnSlot("SES", turnqueue.KindWorker, "worker görevi")
	go func() {
		rel, _ := rt.ClaimSessionCommandTurn(context.Background(), "SES", "/compact")
		rel()
	}()
	waitForTurnWaiters(t, rt, "SES", 1)

	snap := rt.TurnQueue().Snapshot("SES")
	if snap.Running == nil || snap.Running.Kind != turnqueue.KindWorker {
		t.Fatalf("running = %+v, want the worker turn", snap.Running)
	}
	if len(snap.Waiting) != 1 || snap.Waiting[0].Kind != turnqueue.KindCommand {
		t.Fatalf("waiting = %+v, want the queued /compact", snap.Waiting)
	}
	hold()
}
