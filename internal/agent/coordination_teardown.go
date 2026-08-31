package agent

import (
	"context"
	"fmt"
)

// Reversible worker teardown for session deletion.
//
// Deleting a coordinator branch has to stop live worker turns BEFORE the durable
// delete, or a cancelled turn writes into a session that no longer exists. But the
// durable delete can still fail, so stopping must be reversible: preparation marks
// the branch, cancels the in-flight turns and holds their queued follow-ups, and
// the caller then commits (discard) or aborts (restore) once the DB has decided.
//
// The pairing is StopWorkerForTeardown -> FinishWorkerTeardown, and every exit
// path of the former either leaves the branch marked for the caller to finish or
// clears the mark itself.

// StopWorkerForTeardown stops worker-owned runtime state before session deletion.
// Ordinary chat sessions have no worker lifecycle and are already quiesced by the
// chat-run barrier, so they are a successful no-op here.
func (r *Runtime) StopWorkerForTeardown(ctx context.Context, sessionID string) error {
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session %s not found: %w", sessionID, err)
	}
	if !sess.IsWorker() && !sess.IsCoordinator() {
		return nil
	}
	rootID := sess.RootCoordinator()
	if rootID == "" {
		rootID = sess.ID
	}
	unlockTree := lockCoordinatorTree(rootID)
	r.markTreeTeardown(rootID, sessionID)

	targets := []string{sessionID}
	if sess.IsCoordinator() {
		workers, listErr := r.ListSubtreeWorkers(ctx, sessionID)
		if listErr != nil {
			r.clearTreeTeardown(sessionID)
			unlockTree()
			return fmt.Errorf("list worker subtree %s for teardown: %w", sessionID, listErr)
		}
		for _, worker := range workers {
			targets = append(targets, worker.SessionID)
		}
	}
	// Flagging and cancelling under workerQueueMu makes the branch's stop one atomic
	// snapshot against SendToWorker and drainWorkerQueue: a follow-up either landed
	// before the flag (and is held for the commit/abort decision) or is refused.
	controls := make([]*workerCtl, 0, len(targets))
	r.workerQueueMu.Lock()
	for _, target := range targets {
		if v, running := r.workerCancels.Load(target); running {
			ctl := v.(*workerCtl)
			ctl.teardown.Store(true)
			ctl.cancel()
			controls = append(controls, ctl)
		}
	}
	r.workerQueueMu.Unlock()
	unlockTree()
	// Waiting happens with no lock held: a worker turn's exit path takes
	// workerQueueMu itself to drain, so blocking under it would deadlock.
	for _, ctl := range controls {
		if ctl.done == nil {
			continue
		}
		select {
		case <-ctl.done:
		case <-ctx.Done():
			// The deadline passed with turns still running. Un-flag every worker we
			// prepared so the surviving branch keeps draining normally, and drop the
			// spawn block — the caller aborts the delete on this error.
			for _, prepared := range controls {
				prepared.teardown.Store(false)
			}
			r.clearTreeTeardown(sessionID)
			return fmt.Errorf("worker subtree %s did not stop for teardown: %w", sessionID, ctx.Err())
		}
	}
	return nil
}

// FinishWorkerTeardown resolves reversible delete preparation. Commit discards
// queued follow-ups. Abort reopens draining so a failed durable delete leaves the
// surviving worker session usable.
func (r *Runtime) FinishWorkerTeardown(sessionID string, commit bool) {
	r.clearTreeTeardown(sessionID)
	r.workerQueueMu.Lock()
	if commit {
		delete(r.workerQueue, sessionID)
		r.workerQueueMu.Unlock()
		return
	}
	v, running := r.workerCancels.Load(sessionID)
	if running {
		v.(*workerCtl).teardown.Store(false)
	}
	messages := r.workerQueue[sessionID]
	r.workerQueueMu.Unlock()
	// Still running: its own drain runs on exit and now sees teardown cleared, so
	// delivering here too would double-dispatch the same follow-up.
	if len(messages) == 0 || running {
		return
	}
	// The cancelled turn already exited while preparation held its drain. Reuse
	// SendToWorker's normal idle path to preserve policy, receipts, and depth.
	ctx := context.Background()
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		r.logger.Error("coordination: cannot resume worker after aborted teardown", "session", sessionID, "error", err)
		return
	}
	agent, err := r.db.GetAgent(ctx, sess.AgentID)
	if err != nil {
		r.logger.Error("coordination: cannot resume worker agent after aborted teardown", "session", sessionID, "error", err)
		return
	}
	r.drainWorkerQueue(agent, sessionID, sess.CoordinatorSessionID, nil)
}

// markTreeTeardown records that sessionID's branch of rootID's tree is being torn
// down, so spawnBlockedByTreeTeardown refuses new workers underneath it. Callers
// hold the coordinator tree lock for rootID.
func (r *Runtime) markTreeTeardown(rootID, sessionID string) {
	r.treeTeardownMu.Lock()
	defer r.treeTeardownMu.Unlock()
	if r.treeTeardowns == nil {
		r.treeTeardowns = make(map[string]map[string]bool)
		r.teardownTreeRoots = make(map[string]string)
	}
	branches := r.treeTeardowns[rootID]
	if branches == nil {
		branches = make(map[string]bool)
		r.treeTeardowns[rootID] = branches
	}
	branches[sessionID] = true
	r.teardownTreeRoots[sessionID] = rootID
}

func (r *Runtime) clearTreeTeardown(sessionID string) {
	r.treeTeardownMu.Lock()
	defer r.treeTeardownMu.Unlock()
	rootID := r.teardownTreeRoots[sessionID]
	delete(r.teardownTreeRoots, sessionID)
	if branches := r.treeTeardowns[rootID]; branches != nil {
		delete(branches, sessionID)
		if len(branches) == 0 {
			delete(r.treeTeardowns, rootID)
		}
	}
}

// spawnBlockedByTreeTeardown reports whether parentID sits on (or under) a branch
// currently being torn down. Walking up from the parent catches a grandchild spawn
// under an ancestor whose delete is mid-flight, not just a direct child.
func (r *Runtime) spawnBlockedByTreeTeardown(ctx context.Context, rootID, parentID string) bool {
	r.treeTeardownMu.Lock()
	branches := make(map[string]bool, len(r.treeTeardowns[rootID]))
	for branch := range r.treeTeardowns[rootID] {
		branches[branch] = true
	}
	r.treeTeardownMu.Unlock()
	for current := parentID; current != ""; {
		if branches[current] {
			return true
		}
		sess, err := r.db.GetSession(ctx, current)
		if err != nil {
			return false
		}
		current = sess.CoordinatorSessionID
	}
	return false
}
