package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// supersededPair registers two runs for the same session and activates both, so the
// first is SUPERSEDED by the second: its durable writes are fenced, but its
// goroutine, provider subprocess and tools are still running. Each run's cancel
// closes its channel, which is how the tests observe that it was actually stopped.
func supersededPair(t *testing.T, s *Server, wsID, sessionID string) (older, newer chan struct{}) {
	t.Helper()
	older, newer = make(chan struct{}), make(chan struct{})
	var olderOnce, newerOnce sync.Once
	oldRun := s.runs.register("run-old", sessionID, wsID, func() { olderOnce.Do(func() { close(older) }) })
	s.runs.activate(oldRun)
	newRun := s.runs.register("run-new", sessionID, wsID, func() { newerOnce.Do(func() { close(newer) }) })
	s.runs.activate(newRun)
	if info, live := s.runs.sessionRunInfo(wsID, sessionID); !live || info.RunID != "run-new" {
		t.Fatalf("session info must report only the current generation: live=%v info=%+v", live, info)
	}
	return older, newer
}

// TestStopInflightTurn_CancelsAndWaitsForSupersededRun: a superseded predecessor is
// invisible to sessionRunInfo but very much alive. Teardown must cancel it as well as
// the current run, and must not return until BOTH goroutines have unwound — otherwise
// the session is deleted under a provider subprocess that is still running.
func TestStopInflightTurn_CancelsAndWaitsForSupersededRun(t *testing.T) {
	s := &Server{runs: newChatRuns()}
	olderCancelled, newerCancelled := supersededPair(t, s, "WS", "SES")

	var olderFinished, newerFinished atomic.Bool
	// Each turn goroutine returns only after its own context is cancelled, and takes a
	// moment to do so — the window where a stop that does not wait reports success.
	unwind := func(id string, cancelled <-chan struct{}, finished *atomic.Bool) {
		go func() {
			<-cancelled
			time.Sleep(20 * time.Millisecond)
			finished.Store(true)
			s.runs.unregister(id)
		}()
	}
	unwind("run-old", olderCancelled, &olderFinished)
	unwind("run-new", newerCancelled, &newerFinished)

	if err := s.stopInflightTurn("WS", "SES", nil, 2*time.Second); err != nil {
		t.Fatalf("want stopped, got error: %v", err)
	}
	if !olderFinished.Load() {
		t.Fatal("stop returned before the superseded run finished unwinding")
	}
	if !newerFinished.Load() {
		t.Fatal("stop returned before the current run finished unwinding")
	}
	if left := s.runs.sessionRuns("WS", "SES"); len(left) != 0 {
		t.Fatalf("runs still registered after stop: %d", len(left))
	}
}

// TestStopInflightTurn_TimesOutOnStuckSupersededRun: when the superseded run ignores
// its cancellation, teardown must FAIL (so the delete aborts) and name that run —
// reporting success because the visible current run stopped is exactly the bug.
func TestStopInflightTurn_TimesOutOnStuckSupersededRun(t *testing.T) {
	s := &Server{runs: newChatRuns()}
	_, newerCancelled := supersededPair(t, s, "WS", "SES")
	// Only the current run unwinds; "run-old" never unregisters.
	go func() {
		<-newerCancelled
		s.runs.unregister("run-new")
	}()

	err := s.stopInflightTurn("WS", "SES", nil, 100*time.Millisecond)
	if err == nil {
		t.Fatal("want a timeout error while the superseded run is still running")
	}
	if !strings.Contains(err.Error(), "run-old") {
		t.Fatalf("error must name the run that did not stop, got: %v", err)
	}
}

// TestSessionControlStopCancelsSupersededRun: pressing "Durdur" stops the session,
// not just the run the Session Info panel happens to show — a superseded run left
// alive keeps burning tokens and running tools after the user asked for a stop.
func TestSessionControlStopCancelsSupersededRun(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	session, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	olderCancelled, newerCancelled := supersededPair(t, s, wsp.ID, session.ID)

	if rec := postControl(t, s, wsp, session.ID); rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	select {
	case <-olderCancelled:
	default:
		t.Fatal("stop left the superseded run running")
	}
	select {
	case <-newerCancelled:
	default:
		t.Fatal("stop did not cancel the current run")
	}
}
