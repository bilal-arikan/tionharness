// Coordinator tree budget: the per-root live-worker cap shared by every sub-coordinator under one root, the tree lock that serialises admission checks, and the helpers that decide which sessions count against it.
package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// coordinatorTreeLock is one tree's spawn lock plus the number of callers that
// currently hold or are waiting for it. The count is what makes removal safe: the
// entry may only leave the registry once nobody can still be blocked on this exact
// mutex, otherwise two callers would serialize on two different mutexes.
type coordinatorTreeLock struct {
	mu   sync.Mutex
	refs int // guarded by coordinatorTreeLocksMu
}

var coordinatorTreeLocks = map[string]*coordinatorTreeLock{}

// lockCoordinatorTree serializes budget-check-plus-create for one coordinator tree
// and returns the unlock func. The unlock func must be called exactly once.
func lockCoordinatorTree(rootID string) func() {
	coordinatorTreeLocksMu.Lock()
	entry := coordinatorTreeLocks[rootID]
	if entry == nil {
		entry = &coordinatorTreeLock{}
		coordinatorTreeLocks[rootID] = entry
	}
	entry.refs++
	coordinatorTreeLocksMu.Unlock()

	entry.mu.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			entry.mu.Unlock()
			coordinatorTreeLocksMu.Lock()
			entry.refs--
			if entry.refs == 0 && coordinatorTreeLocks[rootID] == entry {
				delete(coordinatorTreeLocks, rootID)
			}
			coordinatorTreeLocksMu.Unlock()
		})
	}
}

// liveWorkerRef names one worker that still occupies a tree-budget slot, for the
// exhaustion error's "still active" listing.
type liveWorkerRef struct {
	SessionID  string
	AgentName  string
	Delegating bool // live only via its own running branch, not a turn of its own
}

// coordTreeBudget is a snapshot of a coordinator tree's LIVE-worker occupancy
// against its ceiling. A worker counts while it can still do or spawn work; a
// worker that has concluded, failed, or been stopped is reclaimed and no longer
// counts — which is exactly what the exhaustion error has always promised
// ("conclude existing workers before spawning more"). total <= 0 means unlimited.
type coordTreeBudget struct {
	used  int
	total int
	live  []liveWorkerRef
}

// exhausted reports whether a further worker would exceed the ceiling.
func (b coordTreeBudget) exhausted() bool { return b.total > 0 && b.used >= b.total }

// activeList renders the still-counted workers for the exhaustion error, so a
// coordinator learns WHICH workers hold the budget rather than only that it is
// full.
func (b coordTreeBudget) activeList() string {
	if len(b.live) == 0 {
		return "No workers are currently counted as active."
	}
	var sb strings.Builder
	sb.WriteString("Still counted as active: ")
	for i, w := range b.live {
		if i > 0 {
			sb.WriteString(", ")
		}
		role := ""
		if w.Delegating {
			role = " (delegating sub-coordinator)"
		}
		fmt.Fprintf(&sb, "%s%s [%s]", w.AgentName, role, w.SessionID)
	}
	sb.WriteString(".")
	return sb.String()
}

// countsAgainstTreeBudget reports whether a worker session still occupies a slot
// in its tree's budget. Mirrors workerInfoFor's liveness (a running turn, or a
// sub-coordinator whose own branch is still live) without the message fetch, so
// it is cheap to call for every node while holding the tree lock. A
// concluded/failed/stopped worker returns false and is reclaimed.
func (r *Runtime) countsAgainstTreeBudget(s db.Session) bool {
	if r.isSessionActive(s.ID) {
		return true
	}
	if s.IsCoordinator() && r.subCoordinatorBusy(s) {
		return true
	}
	return false
}

// subCoordinatorBusy reports whether a sub-coordinator with NO turn of its own in
// flight is nevertheless still occupied. Two independent signals, either of which
// means "not idle":
//
//   - its own workers are running (the in-memory slot counter), or
//   - it still owes its coordinator a report (the persisted
//     CoordinatorReportPending flag) — the state a node sits in between the turn
//     that fanned its work out and the turn that finally synthesizes it.
//
// The second one is load-bearing since the interim "delegating" note was removed:
// with the note gone, a node whose workers have all finished but which has not yet
// reported would otherwise read as FINISHED in its parent's live view, and the
// parent would conclude on a branch that has produced no result.
func (r *Runtime) subCoordinatorBusy(s db.Session) bool {
	return s.CoordinatorReportPending || r.coordSlotFor(s.ID).workers.Load() > 0
}

// evalCoordinatorTreeBudget walks the whole tree and counts the workers that are
// still LIVE (see countsAgainstTreeBudget). Finished workers are reclaimed rather
// than held forever, so a long-running coordinator is not permanently bricked by
// the sessions of work it already completed. total <= 0 short-circuits (no walk).
func (r *Runtime) evalCoordinatorTreeBudget(ctx context.Context, parent db.Session, rootID string) (coordTreeBudget, error) {
	b := coordTreeBudget{total: r.tun.CoordinatorMaxSubtreeSessions()}
	if b.total <= 0 {
		return b, nil
	}
	tree, err := r.db.ListCoordinatorTree(ctx, rootID)
	if err != nil {
		// The root is gone (deleted mid-run). Fall back to the parent's own subtree
		// so the budget still bites rather than silently disappearing.
		if tree, err = r.db.ListCoordinatorTree(ctx, parent.ID); err != nil {
			return coordTreeBudget{}, fmt.Errorf("cannot verify coordinator tree budget: %w", err)
		}
	}
	for _, s := range tree {
		// The root has no coordinator parent; everything below it is a worker. (A
		// worker always carries CoordinatorSessionID, so this cleanly skips the root
		// regardless of which node the walk normalized to.)
		if s.CoordinatorSessionID == "" {
			continue
		}
		if !r.countsAgainstTreeBudget(s) {
			continue // concluded/failed/stopped: reclaimed, no longer holds a slot
		}
		b.used++
		b.live = append(b.live, liveWorkerRef{
			SessionID:  s.ID,
			AgentName:  r.agentName(s.AgentID),
			Delegating: !r.isSessionActive(s.ID), // live only through its branch
		})
	}
	return b, nil
}

// checkCoordinatorTreeBudget enforces the TREE-WIDE guards before a worker session
// is created: nesting depth and the live-worker count of the whole tree. Both fail
// loudly — a caller that hits a ceiling gets an error naming the limit, never a
// quietly downgraded worker, because a coordinator that believes it delegated work
// it did not delegate stalls waiting for a report that will never come. On success
// it returns the budget snapshot so the caller can report remaining quota.
func (r *Runtime) checkCoordinatorTreeBudget(ctx context.Context, parent db.Session, rootID string, depth int, wantCoordinator bool) (coordTreeBudget, error) {
	if maxDepth := r.tun.CoordinatorMaxDepth(); maxDepth > 0 && depth > maxDepth {
		return coordTreeBudget{}, fmt.Errorf("coordinator depth limit reached (max %d levels; this worker would sit at depth %d). Do this work in the current session, or ask your own coordinator to restructure the plan", maxDepth, depth)
	}
	// Spawning a NON-coordinator leaf at the last allowed level is fine; only the
	// sub-coordinator itself needs room for a level below it.
	if wantCoordinator {
		if maxDepth := r.tun.CoordinatorMaxDepth(); maxDepth > 0 && depth >= maxDepth {
			return coordTreeBudget{}, fmt.Errorf("cannot spawn a sub-coordinator at depth %d: its own workers would exceed the coordinator depth limit (max %d). Spawn a plain worker here instead", depth, maxDepth)
		}
	}
	budget, err := r.evalCoordinatorTreeBudget(ctx, parent, rootID)
	if err != nil {
		return coordTreeBudget{}, err
	}
	if budget.exhausted() {
		return budget, fmt.Errorf("coordinator tree budget exhausted (%d/%d LIVE worker sessions across the whole tree). %s Stop or conclude a still-running worker before spawning more (finished workers are already reclaimed)", budget.used, budget.total, budget.activeList())
	}
	return budget, nil
}
