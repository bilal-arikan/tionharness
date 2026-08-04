package api

import "context"

// coordinatorWorkersBusy reports whether sessionID is a coordinator that still has
// at least one worker running (directly, or a sub-coordinator delegating to its own
// live branch). The inbox worker consults this to HOLD a user message that was
// enqueued while workers run: instead of dispatching it immediately (which would run
// a coordinator turn mid worker-run and flash the queue chip away), the message
// stays in the tray until the workers drain. A plain, non-coordinator session — or a
// coordinator whose workers have all finished — reports false and dispatches at once,
// so this never changes the fast path.
//
// Cost is bounded: a non-coordinator bails after a single in-memory GetSession; a
// coordinator additionally scans its direct children, which is cheap and only runs on
// a coordinator's turn dispatch (infrequent).
// holdForCoordinatorWorkers is the seam the inbox worker calls: it routes to the
// injected fake in tests, or the real probe below in production.
func (s *Server) holdForCoordinatorWorkers(wsID, sessionID string) bool {
	if s.workersBusyFn != nil {
		return s.workersBusyFn(wsID, sessionID)
	}
	return s.coordinatorWorkersBusy(wsID, sessionID)
}

func (s *Server) coordinatorWorkersBusy(wsID, sessionID string) bool {
	if s.workspaces == nil {
		return false
	}
	wsp := s.workspaces.Default()
	if wsID != "" {
		if w, err := s.workspaces.Get(wsID); err == nil {
			wsp = w
		}
	}
	if wsp == nil || wsp.DB == nil || wsp.Runtime == nil {
		return false
	}
	sess, err := wsp.DB.GetSession(context.Background(), sessionID)
	if err != nil || !sess.IsCoordinator() {
		return false
	}
	workers, err := wsp.Runtime.ListWorkers(context.Background(), sessionID)
	if err != nil {
		return false
	}
	for _, wk := range workers {
		// Running = a live turn on the worker; Delegating = a sub-coordinator waiting
		// on its own branch. Both mean the coordinator's work is still in progress.
		if wk.Running || wk.Delegating {
			return true
		}
	}
	return false
}

// kickCoordinatorChain re-kicks the send-queue of sessionID AND every coordinator
// ancestor above it, so a message parked on any level of a coordinator tree resumes
// the moment a worker anywhere below it finishes. A worker completion event only
// carries its DIRECT coordinator id, but the message may be parked on an ancestor
// (a top coordinator waiting on a delegating sub-coordinator's deep branch), so we
// walk the CoordinatorSessionID chain to the root. Each kickInbox is idempotent and
// a no-op when that session has no held items, so kicking the whole chain is cheap.
// The walk is bounded by the tree's max depth and guarded against cycles.
func (s *Server) kickCoordinatorChain(wsID, sessionID string) {
	if s.workspaces == nil {
		s.kickInbox(sessionID)
		return
	}
	wsp := s.workspaces.Default()
	if wsID != "" {
		if w, err := s.workspaces.Get(wsID); err == nil {
			wsp = w
		}
	}
	seen := make(map[string]bool)
	for sid := sessionID; sid != "" && !seen[sid]; {
		seen[sid] = true
		s.kickInbox(sid)
		if wsp == nil || wsp.DB == nil {
			return
		}
		sess, err := wsp.DB.GetSession(context.Background(), sid)
		if err != nil {
			return
		}
		sid = sess.CoordinatorSessionID // climb to the parent coordinator
	}
}
