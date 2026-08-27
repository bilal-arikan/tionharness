package api

import (
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Session write guards. "Read-only" is not one rule but two, both defined once in
// internal/db (IsWritableSessionKind / IsImmutableSessionKind) so the API, the
// runtime and the frontend cannot drift apart:
//
//  1. NOT WRITABLE — no new USER TURN may be started. Task, flow, automation,
//     flow-coordinator and worker transcripts are orchestrator-owned run logs:
//     fully readable, but a fresh user turn has no run to attach to. This mirrors
//     what the UI already does by hiding the composer.
//
//     "schedule" is NOT in that group, though the scheduler writes into it too. It
//     is not a per-run log but the agent's single long-lived cron thread, and the
//     user is meant to keep talking in it between ticks — answer what a scheduled
//     turn asked, correct it, add context for the next fire. Interleaving is not a
//     risk: a user turn and a scheduled turn claim the same per-session turn slot
//     (turnqueue), which serializes them.
//  2. IMMUTABLE — the transcript can never change at all. Currently only the
//     machine-written kinds (insight scan records), where no turn ever runs.
//
// The split matters: stop / steer and answering an ask_user interaction are
// parts of an ALREADY RUNNING turn. Those are legitimate on a task or flow
// session and locking them would break real orchestration flows, so they are
// gated by the immutable guard only. An insight session trips both guards
// anyway, since it runs no turn at all.
//
// Rewind is deliberately NOT in that group. It does not steer the live turn: it
// truncates the transcript at a past checkpoint so the user can re-drive the
// conversation from there. On an orchestrator-owned run log that is a
// destructive edit of a record nobody can re-drive — the composer is hidden, so
// there is no way to continue from the checkpoint — hence it is gated by the
// weaker NOT-WRITABLE guard (rejectReadOnlySession).
//
// Scope: these guards sit on HTTP entry points only. In-process producers
// (the send_message tool in internal/agent/agentmsg.go, automation delivery,
// coordinator→worker messages) write through the store/runtime directly and are
// intentionally NOT gated here — they are the system driving its own sessions.

// rejectNonWritableSession refuses a request that would start a NEW user turn in
// a session whose kind accepts none. It writes the HTTP error itself when it
// rejects, and reports whether it did.
//
// A session that cannot be loaded (missing id, unknown session, store error) is
// NOT rejected here: the caller's own handler owns that failure and reports it
// with its own status. This guard only ever adds a refusal, never hides one.
func (s *Server) rejectNonWritableSession(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	sess, ok := s.guardSession(r, sessionID)
	if !ok || db.IsWritableSessionKind(sess.Kind) {
		return false
	}
	reason := "sessions of kind " + quoteKind(sess.Kind) + " accept no new user messages: " +
		"the transcript is written by the orchestrator and a new user turn has no run to attach to"
	if db.IsImmutableSessionKind(sess.Kind) {
		reason = "sessions of kind " + quoteKind(sess.Kind) +
			" are a read-only record of a completed automated run and accept no messages at all"
	}
	writeError(w, http.StatusForbidden, "refusing to enqueue a user message — "+reason+
		" (session "+sessionID+", kind "+quoteKind(sess.Kind)+")")
	return true
}

// rejectReadOnlySession refuses a request that would MODIFY the transcript of a
// session whose kind accepts no user turn (a rewind, today). The message names
// the operation so the caller does not have to re-word the writable rule.
func (s *Server) rejectReadOnlySession(w http.ResponseWriter, r *http.Request, sessionID string, operation string) bool {
	sess, ok := s.guardSession(r, sessionID)
	if !ok || db.IsWritableSessionKind(sess.Kind) {
		return false
	}
	reason := "sessions of kind " + quoteKind(sess.Kind) +
		" are a read-only run log written by the orchestrator: the transcript is a record of what ran," +
		" and with no way to start a new user turn there is nothing to re-drive from the checkpoint"
	if db.IsImmutableSessionKind(sess.Kind) {
		reason = "sessions of kind " + quoteKind(sess.Kind) +
			" are a read-only record of a completed automated run and can never be modified"
	}
	writeError(w, http.StatusForbidden, "refusing "+operation+" — "+reason+
		" (session "+sessionID+")")
	return true
}

// rejectImmutableSession refuses a request that would touch a running turn
// (stop/steer, interaction answer) in a session that never runs one and
// whose transcript must never change.
func (s *Server) rejectImmutableSession(w http.ResponseWriter, r *http.Request, sessionID string, operation string) bool {
	sess, ok := s.guardSession(r, sessionID)
	if !ok || !db.IsImmutableSessionKind(sess.Kind) {
		return false
	}
	writeError(w, http.StatusForbidden, "refusing "+operation+
		" — sessions of kind "+quoteKind(sess.Kind)+" are a read-only record of a completed automated run: "+
		"no turn ever runs in them and the transcript can never be modified (session "+sessionID+")")
	return true
}

// guardSession loads the session a guard is about to judge. ok is false when
// there is nothing to judge (no id, no workspace, unreadable session).
func (s *Server) guardSession(r *http.Request, sessionID string) (db.Session, bool) {
	if sessionID == "" {
		return db.Session{}, false
	}
	wsp := ws(r)
	if wsp == nil || wsp.DB == nil {
		return db.Session{}, false
	}
	sess, err := wsp.DB.GetSession(r.Context(), sessionID)
	if err != nil {
		return db.Session{}, false
	}
	return sess, true
}

// quoteKind renders a Session.Kind for an error message; the manual-chat kind is
// the empty string, which would otherwise read as a hole in the sentence.
func quoteKind(kind string) string {
	if kind == "" {
		return `"" (chat)`
	}
	return kind
}
