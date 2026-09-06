package api

import (
	"net/http"
	"strings"
)

// Converting a WAITING queued message into live guidance for the turn that is
// already running: POST /api/sessions/{id}/queue/{msgId}/steer.
//
// Why one endpoint instead of the client calling "steer" then "cancel queued":
// between those two calls the serial worker can dispatch the very message being
// converted, so the text would run BOTH as guidance and as its own turn — or,
// with the calls in the other order, vanish from the queue after the steer was
// rejected. Neither is recoverable client-side.

// queuedSteerOutcome is the result of converting one queued message. Every value
// except queuedSteerConverted leaves the message exactly where it was.
type queuedSteerOutcome int

const (
	// queuedSteerNotFound: no WAITING message with that id (already dispatched,
	// cancelled, or never queued).
	queuedSteerNotFound queuedSteerOutcome = iota
	// queuedSteerAttachments: the message carries uploads, which a steer cannot
	// convey — converting it would silently drop them.
	queuedSteerAttachments
	// queuedSteerEmpty: the message has no text to steer with.
	queuedSteerEmpty
	// queuedSteerUnsupported: the running turn has no boundary a steer can ride.
	queuedSteerUnsupported
	// queuedSteerBufferFull: the turn is not consuming guidance.
	queuedSteerBufferFull
	// queuedSteerConverted: the guidance reached the run AND the message left the
	// queue — the only outcome that mutates anything.
	queuedSteerConverted
)

// steerQueuedMessage moves a waiting message from the queue into the running
// turn as live guidance, atomically.
//
// Atomicity comes from doing BOTH halves under the single inbox lock: the serial
// worker pops the head under that same lock (popInboxHead), so it cannot dispatch
// the message while we hold it, and the removal happens only after the run has
// accepted the text. A rejected steer therefore leaves the queue untouched — the
// user's message is never lost — and an accepted one can never also run as its
// own turn.
//
// The callback deliberately does more than mutate the inbox (see withInbox's
// contract): it hands the text to the run. That is the whole point — both
// deliverSteer paths are non-blocking and take only the run's own mutex, so they
// neither block the queue nor re-enter the inbox lock.
//
// A run that finishes in the same instant is still safe: the guidance it never
// consumed is rescued at turn end by recoverUndeliveredSteer, which re-queues it
// at the FRONT.
func (s *Server) steerQueuedMessage(wsID, sessionID, clientMsgID string, run *chatRun) queuedSteerOutcome {
	outcome := queuedSteerNotFound
	s.withInbox(wsID, sessionID, func(ib *sessionInbox) bool {
		for idx, it := range ib.items {
			if it.ClientMsgID != clientMsgID {
				continue
			}
			if len(it.Req.Attachments) > 0 {
				outcome = queuedSteerAttachments
				return false
			}
			text := it.Req.Message
			if strings.TrimSpace(text) == "" {
				outcome = queuedSteerEmpty
				return false
			}
			switch deliverSteer(run, text) {
			case steerUnsupported:
				outcome = queuedSteerUnsupported
				return false
			case steerBufferFull:
				outcome = queuedSteerBufferFull
				return false
			}
			ib.items = append(ib.items[:idx:idx], ib.items[idx+1:]...)
			outcome = queuedSteerConverted
			return true
		}
		return false
	})
	return outcome
}

// handleSteerQueued converts a queued message into a steer for the session's
// in-flight turn. Every refusal keeps the message queued and says why, so the
// user can leave it to run as a normal turn instead.
func (s *Server) handleSteerQueued(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	msgID := r.PathValue("msgId")
	if sessionID == "" || msgID == "" {
		writeError(w, http.StatusBadRequest, "session id and message id required")
		return
	}
	if s.rejectImmutableSession(w, r, sessionID, "session control (stop/steer)") {
		return
	}
	run, reason := s.steerTargetRun(ws(r).ID, sessionID)
	if run == nil {
		writeError(w, http.StatusNotFound, reason)
		return
	}
	switch s.steerQueuedMessage(ws(r).ID, sessionID, msgID, run) {
	case queuedSteerNotFound:
		writeError(w, http.StatusNotFound, "no waiting queued message with id "+msgID+" (already dispatched or cancelled?)")
	case queuedSteerAttachments:
		writeError(w, http.StatusBadRequest, "queued message has attachments; a steer carries text only — it stays in the queue")
	case queuedSteerEmpty:
		writeError(w, http.StatusBadRequest, "queued message has no text to steer with — it stays in the queue")
	case queuedSteerUnsupported:
		// Same contract as the control endpoint: 200 with an explicit result, so the
		// client can show the "this turn cannot be steered" hint. The message is
		// still queued and will run as its own turn.
		writeJSON(w, http.StatusOK, map[string]string{"result": "unsupported"})
	case queuedSteerBufferFull:
		writeError(w, http.StatusServiceUnavailable, steerBufferFullMsg)
	case queuedSteerConverted:
		writeJSON(w, http.StatusOK, map[string]string{"result": "steered"})
	}
}
