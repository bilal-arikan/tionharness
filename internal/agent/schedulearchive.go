package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// Archiving a session is the user's STOP gesture, and a schedule bound to that
// session is the one thing that could ignore it: the cron table keeps firing,
// each fire claims a turn on a session nobody is watching, and the output lands
// in a thread that has left the active list. The user archived the session
// believing the work was over while it quietly kept running.
//
// The fix is at the source — when a session is archived, the schedules bound to
// it are disabled — so the schedule stops firing AND the user can SEE it is off
// in the schedules view. This is deliberately not the same lever as session
// reactivation (sessionreactivate.go): a wake must not RESURRECT an archived
// session, so KindWake stays off the reactivating allowlist there and the turn
// is refused outright in turnslot.go. Here we stop the fires from ever being
// scheduled again; there we catch the one already in flight.
//
// Disabled, not archived: Enabled is the field the cron table reads
// (ListEnabledSchedules) and the field the UI renders as an on/off switch, so
// flipping it is both the effective stop and the visible one. Restoring the
// session does not re-enable the schedules — re-arming automatic work is an
// explicit decision for the user, not a side effect of un-archiving.

// onSessionArchivedDisableSchedules disables every schedule bound to a session
// that has just been archived. Wired into OnSessionChange, so it covers every
// archive path at once — the HTTP endpoint, update_session, the archive_sessions
// bulk sweep, the coordinator's terminal-worker archive and the insight sweeper
// all funnel through db.SetSessionState.
//
// Only the active→archived edge does work; any other transition (including an
// idempotent re-archive of an already archived row) returns immediately, so the
// common path costs one string compare.
func (r *Runtime) onSessionArchivedDisableSchedules(ev db.SessionChangeEvent) {
	if ev.Op != db.SessionOpState || ev.Session.State != "archived" || ev.PrevState == "archived" {
		return
	}
	// Detached from any request context: the archive write already succeeded, and
	// the stop must not be skipped because the caller's context is being cancelled.
	ctx := context.Background()
	schedules, err := r.db.ListSchedules(ctx)
	if err != nil {
		// A store failure here means we cannot tell whether a schedule survived the
		// archive — exactly the silent-continuation case this code exists to prevent.
		r.logger.Warn("archive: schedule lookup failed, schedules may still fire",
			"session", ev.SessionID, "error", err)
		return
	}
	disabled := 0
	for _, sc := range schedules {
		if !sc.Enabled || !scheduleBoundToSession(sc, ev.Session) {
			continue
		}
		if err := r.db.SetScheduleEnabled(ctx, sc.ID, false); err != nil {
			r.logger.Warn("archive: disabling a bound schedule failed, it keeps firing",
				"session", ev.SessionID, "schedule", sc.ID, "error", err)
			continue
		}
		disabled++
		r.logger.Info("schedule disabled: its session was archived",
			"session", ev.SessionID, "schedule", sc.ID, "oneShot", sc.OneShot)
	}
	if disabled == 0 {
		return
	}
	// Rebuild the cron table so the disabled rows lose their timers now rather
	// than at the next reload — a one-shot wake timer is already armed and would
	// otherwise still fire. Nil-safe before the scheduler exists (boot, tests).
	if err := r.reloadSchedules(ctx); err != nil {
		r.logger.Warn("archive: scheduler reload failed, a disabled schedule may still be armed",
			"session", ev.SessionID, "error", err)
	}
}

// ErrSessionArchived refuses a scheduled turn on an archived session. Callers
// (the scheduler's wake and scheduled-prompt paths) already treat a failed slot
// claim as "the turn never started, nothing was recorded", which is exactly the
// outcome wanted here.
var ErrSessionArchived = errors.New("session is archived: scheduled turn skipped")

// refuseWakeOnArchivedSession is the safety net behind the real fix in
// onSessionArchivedDisableSchedules. A schedule disabled at archive time cannot
// fire again, but two cases still reach the turn queue: a fire already in flight
// when the session was archived, and a row that escaped the sweep (a store error
// above, or a binding added later that scheduleBoundToSession does not know).
// Rather than let either write into a session the user archived, the claim is
// refused before the queue is touched.
//
// Refused, never resurrected — reviving on a wake would restart the runaway
// coordinator loop archiving exists to break (see reactivatingKinds), so
// KindWake stays off that allowlist and dies here instead.
//
// The skip is LOGGED, not swallowed: a turn silently vanishing is precisely the
// failure the user reported, just with the sign flipped. Only KindWake is
// gated; every other kind keeps the behaviour sessionreactivate.go defines.
func (r *Runtime) refuseWakeOnArchivedSession(ctx context.Context, sessionID string, kind turnqueue.Kind) error {
	if sessionID == "" || kind != turnqueue.KindWake {
		return nil
	}
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		// An unknown session is the caller's own problem to report — it loads the
		// session right after claiming and fails loudly there. Anything else is a
		// real store failure and must not silently gate a turn off.
		if !errors.Is(err, db.ErrNotFound) {
			r.logger.Warn("wake gate: session lookup failed", "session", sessionID, "error", err)
		}
		return nil
	}
	if sess.State != "archived" {
		return nil
	}
	r.logger.Warn("scheduled turn skipped: its session is archived",
		"session", sessionID, "kind", string(kind), "agent", sess.AgentID)
	return fmt.Errorf("%w (session %s, turn kind %s)", ErrSessionArchived, sessionID, string(kind))
}

// scheduleBoundToSession reports whether firing sc would deliver a turn INTO
// sess. Two bindings exist, matching the scheduler's two delivery paths:
//
//   - SessionID set — a one-shot wake re-delivers its prompt into that exact
//     session (deliverWake).
//   - reuse mode — a recurring schedule appends to its agent's shared "schedule"
//     thread, the single session with that agent id and kind (deliverPrompt via
//     GetOrCreateKindSession).
//
// Spawn-mode schedules are deliberately NOT bound: each fire opens its own fresh
// session, so archiving one past run says nothing about the schedule itself.
func scheduleBoundToSession(sc db.Schedule, sess db.Session) bool {
	if sc.SessionID != "" {
		return sc.SessionID == sess.ID
	}
	if sc.OneShot {
		// A one-shot with no session has nothing to re-enter; it is not bound to any.
		return false
	}
	return sess.Kind == "schedule" && sess.AgentID == sc.AgentID &&
		sc.EffectiveSessionMode() == db.ScheduleSessionModeReuse
}
