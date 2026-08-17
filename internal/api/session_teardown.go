package api

import (
	"context"
	"fmt"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// sessionTeardownGrace bounds how long delete waits for an in-flight turn to actually
// unwind after it is cancelled. The run's cancel is the turn ctx, so a claude-cli
// subprocess is torn down on cancel and the turn goroutine returns quickly; a turn
// still alive after this window is treated as un-stoppable and BLOCKS the delete
// rather than being stranded against a session that no longer exists.
const sessionTeardownGrace = 15 * time.Second

// teardownSessionRuntime stops everything runtime-side tied to a session BEFORE it is
// deleted: the in-flight turn (and its claude-cli subprocess), warm pooled claude-cli
// processes, the serial inbox worker, and any autonomous worker turn. It is FAIL
// CLOSED — if a live process/turn cannot be stopped it returns an error and leaves the
// session intact (queue restored, worker resumed), so the caller aborts the delete
// instead of orphaning a process that outlives its session.
//
// Ordering matters: the inbox is frozen first (so its worker cannot dispatch a NEW
// turn mid-teardown) but its queued messages are KEPT, then the live turn is stopped,
// then warm processes are killed. Only once every live process is confirmed down do we
// discard the in-memory queue. The DB row/folder (and the on-disk inbox sidecar inside
// it) are removed by the caller's DB.DeleteSession afterwards.
func (s *Server) teardownSessionRuntime(wsp *workspace.Workspace, sessionID string) error {
	// Every phase below is workspace-scoped (ids repeat across stores), so a missing
	// workspace cannot be worked around — teardown is fail-closed: refuse rather than
	// tear down whatever session happens to carry this id elsewhere.
	if wsp == nil {
		return fmt.Errorf("session teardown requires a workspace")
	}
	wsID := wsp.ID

	// Phase 1: freeze the inbox worker (stop popping new turns; keep the queue).
	s.inbox.lock()
	if ib := s.inbox.at(wsID, sessionID); ib != nil {
		ib.closing = true
	}
	s.inbox.unlock()

	// Phase 2: cancel the in-flight turn and WAIT for it to fully unwind (tears down the
	// subprocess). If it will not stop in time, abort: unfreeze + resume, keep the session.
	if err := s.stopInflightTurn(wsID, sessionID, sessionTeardownGrace); err != nil {
		s.resumeInboxAfterAbortedTeardown(wsID, sessionID)
		return err
	}

	// Phase 3: kill warm claude-cli processes kept between turns, verifying each kill. A
	// process we cannot terminate blocks the delete (fail closed) — it stays tracked in
	// the pool, not orphaned.
	if wsp.Runtime != nil {
		if _, err := wsp.Runtime.DropWarmCLISessionChecked(sessionID); err != nil {
			s.resumeInboxAfterAbortedTeardown(wsID, sessionID)
			return fmt.Errorf("warm claude-cli teardown: %w", err)
		}
	}

	// Past here nothing can fail — commit the in-memory teardown.

	// Phase 4: stop an autonomous worker turn if this session is one — and, when it is
	// a sub-coordinator, its whole subtree with it, so deleting a branch does not leave
	// grandchildren running against a session that no longer exists. "Not running" is
	// the common case (most sessions are not workers) and is not a teardown failure, so
	// its error is intentionally ignored. Empty coordinator id = skip the
	// "is it really yours" ownership check; teardown is authoritative here.
	if wsp.Runtime != nil {
		_ = wsp.Runtime.StopWorker(context.Background(), "", sessionID)
	}

	// Phase 5: drop the in-memory inbox entirely so the serial worker exits for good and
	// never re-dispatches a turn for a deleted session.
	s.inbox.lock()
	delete(s.inbox.sessions, scopeKey(wsID, sessionID))
	s.inbox.unlock()

	// Phase 6: release the session's hub state (seq, replay ring, subscribers).
	// Nothing else ever frees it, so without this every session the process has
	// seen — including the throwaway schedule/spawn/worker ones — keeps its ring
	// buffer alive until restart. Any window still watching gets its channel
	// closed, which ends its stream: correct for a session being deleted.
	s.hub.Drop(wsID, sessionID)

	return nil
}

// stopInflightTurn cancels the session's live turn and blocks until it actually
// finishes (run.done closes on unregister) or the grace window elapses. Returns an
// error only when a turn is still running after grace — the signal that it could not
// be stopped. Loops so a straggler direct/autonomous run settling right after the
// first is also caught; the frozen inbox guarantees no NEW queued turn starts meanwhile.
func (s *Server) stopInflightTurn(wsID, sessionID string, grace time.Duration) error {
	deadline := time.Now().Add(grace)
	for {
		info, live := s.runs.sessionRunInfo(wsID, sessionID)
		if !live {
			return nil
		}
		run := s.runs.get(info.RunID)
		if run == nil {
			return nil // finished between the snapshot and the lookup
		}
		run.cancel()
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("in-flight turn %s did not stop within %s", info.RunID, grace)
		}
		select {
		case <-run.done:
			// Re-check for a straggler run before declaring the session quiet.
		case <-time.After(remaining):
			return fmt.Errorf("in-flight turn %s did not stop within %s", info.RunID, grace)
		}
	}
}

// resumeInboxAfterAbortedTeardown unfreezes a session's inbox and re-kicks its worker
// after a delete was aborted (a process would not die), so messages queued behind the
// freeze drain now that the session lives on.
func (s *Server) resumeInboxAfterAbortedTeardown(wsID, sessionID string) {
	s.inbox.lock()
	if ib := s.inbox.at(wsID, sessionID); ib != nil {
		ib.closing = false
	}
	s.inbox.unlock()
	s.kickInbox(wsID, sessionID)
}
