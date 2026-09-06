package agent

import (
	"context"
	"errors"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// Session reactivation: archiving is a shelf, not a grave. The moment someone —
// a human or another agent — actually addresses an archived session again, it
// belongs back in the active list instead of forcing the user to restore it by
// hand before the reply can be read.
//
// The hook sits on the turn-admission choke point (claimTurnSlot) because that is
// the ONE place every inbound command/message passes through: chat, the queued
// inbox, a peer send_message, a send_to_worker task and a spawn's opening turn
// all claim the slot first. Just as importantly, nothing that merely READS a
// session claims one — opening it, attaching to its stream, generating a title or
// summary, or an insight scan run outside the queue — so admission is the exact
// line between "someone is talking to this session" and "someone is looking at it".

// reactivatingKinds are the turn kinds where a human or an agent explicitly
// ADDRESSES this session: a chat/inbox message, a peer send_message, a worker
// task handed over by send_to_worker (the terminal worker session a coordinator
// archived at coordination.go's report path is exactly the case this card is
// about), and a spawn's opening turn.
//
// An allowlist, not a denylist, so a turn kind added later stays inert until
// somebody decides it should revive a session. What is deliberately absent:
//
//   - KindCommand — /compact and /handoff are maintenance run ON a transcript,
//     not a new instruction TO the agent.
//   - KindCoordinator, KindWake, KindAutomation — automatic re-entries (a drain
//     from a worker notification, a scheduler wake, an automation trigger). For
//     these, archiving is the documented HARD STOP that breaks a runaway loop
//     (see enqueueCoordinatorWake); reviving on them would restart precisely the
//     work the user archived to kill, and a scheduled session could never stay
//     archived at all. The notes they carry stay durably recorded either way, so
//     restoring the session by hand replays them.
var reactivatingKinds = map[turnqueue.Kind]bool{
	turnqueue.KindUser:   true,
	turnqueue.KindPeer:   true,
	turnqueue.KindWorker: true,
	turnqueue.KindSpawn:  true,
}

// reactivateArchivedSession flips an archived session back to "active" when a
// real turn is admitted on it, drops the mirrored "archived" auto-tag and emits
// the same "session"/"state" change event the HTTP and tool archive paths emit,
// so every open window moves the row between the Active/Archived filters at once.
//
// A live (non-archived) session costs one already-cached session read and no
// write. Called after the slot is held, so it can never race the turn it belongs
// to; a turn that archives the session mid-flight is not undone, because the
// claim that would re-activate it already happened.
func (r *Runtime) reactivateArchivedSession(ctx context.Context, sessionID string, kind turnqueue.Kind) {
	if sessionID == "" || !reactivatingKinds[kind] {
		return
	}
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		// An unknown session is the turn's own problem to report — every caller
		// loads the session right after claiming the slot and fails loudly there.
		// Anything else is a real store failure and must not pass unnoticed.
		if !errors.Is(err, db.ErrNotFound) {
			r.logger.Warn("reactivate: session lookup failed", "session", sessionID, "error", err)
		}
		return
	}
	if sess.State != "archived" {
		return
	}
	if err := r.db.SetSessionState(ctx, sessionID, "active"); err != nil {
		r.logger.Warn("reactivate: state write failed", "session", sessionID, "error", err)
		return
	}
	// The "archived" tag mirrors the lifecycle state (autotag.go), so it has to go
	// with it. Dropped regardless of the auto-tag setting: a tag that is now false
	// would otherwise survive a setting that was turned off after it was written.
	r.RemoveSessionTags(ctx, sessionID, []string{TagArchived})
	r.logger.Info("session reactivated by an incoming turn", "session", sessionID, "kind", string(kind))
	r.publish(events.Event{
		Type:   "session",
		Level:  "info",
		Target: map[string]string{"sessionId": sessionID, "op": "state"},
	})
}
