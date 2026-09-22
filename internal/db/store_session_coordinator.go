// Coordinator-mode session fields: mode and workflow, the parent/root lineage, the pending-report flag with its single-claim semantics, and the tree/ancestor listings the coordinator runtime walks.
package db

import (
	"cmp"
	"context"
	"slices"
)

// SetCoordinatorMode turns a session's coordinator capability on or off (M2). It
// touches ONLY CoordinatorMode: Role stays whatever the session's lineage is, so
// enabling it on a worker produces a mid-level node (worker + coordinator) rather
// than severing its link to its parent. Disabling also clears the LEGACY Role
// value, otherwise IsCoordinator() would keep returning true on an old session
// and the toggle would silently do nothing.
func (d *DB) SetCoordinatorMode(ctx context.Context, sessionID string, enabled bool) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.CoordinatorMode = enabled
		if !enabled && s.Role == SessionRoleCoordinator {
			s.Role = ""
		}
	})
}

// SetSessionCoordinatorLineage stamps a freshly spawned worker's place in its
// coordinator tree: its parent, the tree root, and its depth below that root.
// Written once at spawn time, never edited afterwards — the tree shape is fixed
// at creation, which is what makes RootCoordinator()/depth safe to trust for
// tree-wide budgeting.
func (d *DB) SetSessionCoordinatorLineage(ctx context.Context, sessionID, parentID, rootID string, depth int) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.CoordinatorSessionID = parentID
		s.RootCoordinatorSessionID = rootID
		s.CoordinatorDepth = depth
	})
}

// SetCoordinatorReportPending records whether a mid-level node still owes its
// coordinator an upward report (see Session.CoordinatorReportPending).
func (d *DB) SetCoordinatorReportPending(ctx context.Context, sessionID string, pending bool) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.CoordinatorReportPending = pending
	})
}

// ClaimCoordinatorReport atomically takes ownership of a session's outstanding
// upward report: it clears CoordinatorReportPending and returns whether THIS caller
// is the one that flipped it. Only the winner may send the report.
//
// A plain read-then-clear is not enough. Two settle backstops can be armed for the
// same session (one per drain exit), and a backstop can run alongside the agent's
// own report_to_coordinator — each would pass its own "does it still owe one?"
// check and send, so the coordinator above would receive the same task reported
// twice, with different statuses. The store lock makes the flip indivisible.
func (d *DB) ClaimCoordinatorReport(ctx context.Context, sessionID string) (bool, error) {
	tl := d.transcriptLock(sessionID)
	tl.Lock()
	recovered, err := d.recoverCLIReplyBeforeMutationLocked(sessionID)
	if err != nil {
		tl.Unlock()
		return false, err
	}
	defer func() {
		tl.Unlock()
		d.deliverRecoveredCLIReplyActivity(recovered, sessionID)
	}()
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return false, ErrNotFound
	}
	if !s.CoordinatorReportPending {
		return false, nil // someone else already reported
	}
	s.CoordinatorReportPending = false
	return true, d.persistSessionLocked(s)
}

// ListPendingCoordinatorReports returns every non-archived session that still owes
// its coordinator a report. Read at boot to re-arm the settle backstop for nodes
// whose owed report would otherwise be forgotten across a restart.
func (d *DB) ListPendingCoordinatorReports(ctx context.Context) ([]Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var out []Session
	for _, s := range d.sessions {
		if s.CoordinatorReportPending && s.State != "archived" && s.CoordinatorSessionID != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

// ListCoordinatorTree returns every session in the coordinator tree that sessionID
// belongs to, INCLUDING the root and sessionID itself, in breadth-first order from
// the root. Accepts any member of the tree (root, mid-level node, or leaf) and
// normalizes to the root first — the same contract as ListFlowRunTree (_Docs/62),
// so a UI can hand it whatever session the user happens to be looking at.
//
// One pass over the session map builds the parent→children index: walking
// CoordinatorSessionID per node would be O(depth) lookups per node, and this runs
// on every coordinator turn (the live worker-status block).
func (d *DB) ListCoordinatorTree(ctx context.Context, sessionID string) ([]Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	start, ok := d.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	rootID := start.RootCoordinator()
	if rootID == "" {
		return []Session{start}, nil // neither coordinator nor worker: a tree of one
	}
	root, ok := d.sessions[rootID]
	if !ok {
		// The root was deleted out from under its subtree. Treat the caller as the
		// root so the surviving nodes stay reachable — returning an empty tree here
		// would read as "no workers" to a coordinator still waiting on them.
		root, rootID = start, start.ID
	}
	children := map[string][]Session{}
	for _, s := range d.sessions {
		if s.CoordinatorSessionID != "" {
			children[s.CoordinatorSessionID] = append(children[s.CoordinatorSessionID], s)
		}
	}
	out := []Session{root}
	queue := []string{rootID}
	seen := map[string]bool{rootID: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		kids := children[cur]
		sortSessionsByCreation(kids)
		for _, k := range kids {
			if seen[k.ID] {
				continue // defensive: a hand-edited parent cycle must not hang the walk
			}
			seen[k.ID] = true
			out = append(out, k)
			queue = append(queue, k.ID)
		}
	}
	return out, nil
}

// ListCoordinatorAncestors returns the chain from sessionID's ROOT down to its
// direct parent (root first, parent last); empty for a root or an ordinary
// session. This is the breadcrumb a worker walks upward to see who it reports to.
func (d *DB) ListCoordinatorAncestors(ctx context.Context, sessionID string) ([]Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	var chain []Session
	seen := map[string]bool{sessionID: true} // bounds a hand-edited parent cycle
	for cur := s.CoordinatorSessionID; cur != ""; {
		if seen[cur] {
			break
		}
		seen[cur] = true
		p, ok := d.sessions[cur]
		if !ok {
			break
		}
		chain = append(chain, p)
		cur = p.CoordinatorSessionID
	}
	// Collected parent-first; callers want root-first.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// sortSessionsByCreation orders siblings oldest-first so a tree walk is stable
// across calls. CreatedAt has second resolution, so ties fall back to the id
// (monotonic per store) rather than leaving sibling order to map iteration.
func sortSessionsByCreation(ss []Session) {
	slices.SortFunc(ss, func(a, b Session) int {
		if c := cmp.Compare(a.CreatedAt, b.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

// SetSessionCoordinatorWorkflow records the selected coordinator recipe (M5) on a
// session plus the resolved per-session notify-loop cap override (0 = keep the
// workspace default). An empty slug clears the selection.
func (d *DB) SetSessionCoordinatorWorkflow(ctx context.Context, sessionID, slug string, maxTurns int) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.CoordinatorWorkflow = slug
		s.CoordinatorMaxTurns = maxTurns
	})
}
