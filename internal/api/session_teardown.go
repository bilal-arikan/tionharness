package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// sessionTeardownGrace bounds how long delete waits for an in-flight turn to actually
// unwind after it is cancelled. The run's cancel is the turn ctx, so a claude-cli
// subprocess is torn down on cancel and the turn goroutine returns quickly; a turn
// still alive after this window is treated as un-stoppable and BLOCKS the delete
// rather than being stranded against a session that no longer exists.
const sessionTeardownGrace = 15 * time.Second

type sessionTeardownLock struct {
	mu   sync.Mutex
	refs int
}

type sessionTeardownLocks struct {
	mu    sync.Mutex
	locks map[string]*sessionTeardownLock
}

func (l *sessionTeardownLocks) lock(key string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[string]*sessionTeardownLock)
	}
	entry := l.locks[key]
	if entry == nil {
		entry = &sessionTeardownLock{}
		l.locks[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		l.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(l.locks, key)
		}
		l.mu.Unlock()
	}
}

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
	return s.teardownSessionRuntimeWithGrace(wsp, sessionID, sessionTeardownGrace)
}

func (s *Server) teardownSessionRuntimeWithGrace(wsp *workspace.Workspace, sessionID string, grace time.Duration) error {
	// Every phase below is workspace-scoped (ids repeat across stores), so a missing
	// workspace cannot be worked around — teardown is fail-closed: refuse rather than
	// tear down whatever session happens to carry this id elsewhere.
	if wsp == nil {
		return fmt.Errorf("session teardown requires a workspace")
	}
	wsID := wsp.ID
	unlock := s.teardownLocks.lock(scopeKey(wsID, sessionID))
	defer unlock()
	prepared, err := s.prepareSessionRuntimeLocked(wsp, sessionID, grace)
	if err == nil {
		s.finishSessionRuntime(wsp, sessionID, prepared, true)
	}
	return err
}

type preparedSessionRuntime struct {
	worker      bool
	closingRuns []*chatRun
}

// prepareSessionRuntimeLocked requires ownership of the session-scoped teardown
// lock. Its worker stop remains reversible until the delete caller commits it
// after DB.DeleteSession; false is returned when no real runtime was prepared.
func (s *Server) prepareSessionRuntimeLocked(wsp *workspace.Workspace, sessionID string, grace time.Duration) (*preparedSessionRuntime, error) {
	wsID := wsp.ID
	deadline := time.Now().Add(grace)
	deadlineCtx, cancelDeadline := context.WithDeadline(context.Background(), deadline)
	defer cancelDeadline()

	// Phase 1: freeze the inbox worker (stop popping new turns; keep the queue).
	s.inbox.lock()
	if ib := s.inbox.at(wsID, sessionID); ib != nil {
		ib.closing = true
	}
	s.inbox.unlock()

	// Phase 2: reject new bridge calls before provider cancellation. Existing calls
	// stay alive until provider finalization, preserving normal ask-after-turn behavior.
	closingRuns := s.runs.closeSessionCalls(wsID, sessionID)
	abort := func(err error) error {
		s.runs.reopenSessionCalls(closingRuns)
		s.resumeInboxAfterAbortedTeardown(wsID, sessionID)
		return err
	}

	// Phase 3: cancel the in-flight turn and WAIT for it to fully unwind (tears down the
	// subprocess). If it will not stop in time, abort: unfreeze + resume, keep the session.
	if err := s.stopInflightTurn(wsID, sessionID, time.Until(deadline)); err != nil {
		return nil, abort(err)
	}

	// Phase 4: provider finalization does not imply its CLI/MCP handlers returned.
	// Cancel every admitted call and drain them within the SAME teardown deadline.
	for _, done := range s.runs.cancelSessionCalls(closingRuns) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, abort(fmt.Errorf("Interaction MCP calls did not stop within %s", grace))
		}
		timer := time.NewTimer(remaining)
		select {
		case <-done:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			return nil, abort(fmt.Errorf("Interaction MCP calls did not stop within %s", grace))
		}
	}

	// Phase 5: kill warm claude-cli processes kept between turns, verifying each kill. A
	// process we cannot terminate blocks the delete (fail closed) — it stays tracked in
	// the pool, not orphaned.
	if wsp.Runtime != nil {
		if _, err := wsp.Runtime.DropWarmCLISessionChecked(sessionID); err != nil {
			return nil, abort(fmt.Errorf("warm claude-cli teardown: %w", err))
		}
	}

	// Phase 6: stop an autonomous worker turn if this session is one — and, when it is
	// a sub-coordinator, its whole subtree with it, so deleting a branch does not leave
	// grandchildren running against a session that no longer exists. "Not running" is
	// the common case (most sessions are not workers). Empty coordinator id skips the
	// ownership check; teardown is authoritative. This phase shares the teardown
	// deadline because worker cleanup may otherwise write after DB deletion.
	if s.stopWorker != nil {
		if err := s.stopWorker(deadlineCtx, wsp, sessionID); err != nil {
			return nil, abort(fmt.Errorf("worker teardown: %w", err))
		}
	} else if wsp.Runtime != nil {
		if err := wsp.Runtime.StopWorkerForTeardown(deadlineCtx, sessionID); err != nil {
			wsp.Runtime.FinishWorkerTeardown(sessionID, false)
			return nil, abort(fmt.Errorf("worker teardown: %w", err))
		}
	}

	// Phase 7: close this session's SCOPED MCP connections. A scoped stdio server
	// runs as a subprocess whose cwd is the session's scratchpad
	// (agent.applyMCPScratchpadRoot), and Windows refuses to remove a directory
	// that is a live process's cwd — so a still-running server makes the directory
	// removal fail. Runs LAST of the runtime phases: the worker turns above may
	// still be calling MCP tools until they stop.
	//
	// Not reversible and deliberately not fatal: the connection is re-dialled on
	// demand, so an abort after this point costs a re-dial, and a server that
	// cannot be closed is reported by the directory removal that follows rather
	// than pre-emptively blocking the delete here.
	if wsp.Runtime != nil {
		wsp.Runtime.CloseSessionMCP(sessionID)
	}

	return &preparedSessionRuntime{
		worker:      wsp.Runtime != nil && s.stopWorker == nil,
		closingRuns: closingRuns,
	}, nil
}

// finishSessionRuntime commits or rolls back reversible runtime preparation only
// after the durable DB delete has decided the session's fate.
func (s *Server) finishSessionRuntime(wsp *workspace.Workspace, sessionID string, prepared *preparedSessionRuntime, commit bool) {
	if prepared == nil {
		return
	}
	if prepared.worker && wsp.Runtime != nil {
		wsp.Runtime.FinishWorkerTeardown(sessionID, commit)
	}
	if !commit {
		s.runs.reopenSessionCalls(prepared.closingRuns)
		s.resumeInboxAfterAbortedTeardown(wsp.ID, sessionID)
		return
	}
	s.inbox.lock()
	delete(s.inbox.sessions, scopeKey(wsp.ID, sessionID))
	s.inbox.unlock()
	s.hub.Drop(wsp.ID, sessionID)
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
