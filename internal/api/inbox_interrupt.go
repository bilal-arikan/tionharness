package api

import (
	"net/http"
)

// handleInterrupt is the atomic "stop what you are doing and answer THIS instead"
// action behind the composer's Ctrl/Cmd+Enter and the "Kes" button.
//
// Why it is one endpoint instead of two calls: the client used to send
// sessionControl(stop) and then sendMessage(). Between those two round trips the
// session's turn slot is FREE, and it is not only the user who competes for it —
// another queued message, a coordinator auto-turn, a worker <task-notification> or
// a scheduler wake can claim it (see claimRuntimeTurn in the queue worker). The
// interrupt message then ran AFTER the very turn the user interrupted to skip,
// which is the opposite of what the gesture promises.
//
// Order matters and is deliberate: the message is placed at the queue HEAD first,
// and only then is the running turn cancelled. Doing it the other way round
// reopens the same gap — a freed slot with an empty queue is exactly what a
// competing turn needs. Enqueuing first means that whenever the stopped turn
// unwinds, the head is already the user's message.
//
// Enqueuing at the head is itself a single lock hold (enqueueMessageAtHead), not
// an append-then-promote, so the queue worker cannot dispatch between the two
// halves either.
func (s *Server) handleInterrupt(w http.ResponseWriter, r *http.Request, sessionID string, req sessionControlReq) {
	// An attachment-only interrupt is legitimate (the composer allows a turn with
	// files and no text), so the guard is "nothing to send at all".
	if req.Text == "" && len(req.Attachments) == 0 {
		writeError(w, http.StatusBadRequest, "interrupt text or attachments required")
		return
	}
	wsp := ws(r)
	// Enqueue BEFORE stopping (see the ordering note above). A duplicate
	// clientMsgId means this is a retry whose first attempt already landed the
	// message; the stop below still runs, so a retry after a failed stop converges.
	queued := s.enqueueMessageAtHead(wsp.ID, chatReq{
		SessionID:      sessionID,
		Message:        req.Text,
		AgentIDs:       req.AgentIDs,
		ThinkingLevel:  req.ThinkingLevel,
		PermissionMode: req.PermissionMode,
		Attachments:    req.Attachments,
	}, req.ClientMsgID)
	stopped := s.stopSessionTurn(wsp, sessionID)
	// Not an error when nothing was running: the turn may have ended on its own
	// between the user pressing the key and this request arriving. The message is
	// queued either way and runs next, which is what the user asked for — failing
	// here would drop it instead.
	writeJSON(w, http.StatusOK, map[string]any{
		"result":  "ok",
		"queued":  queued,
		"stopped": stopped,
	})
}
