package api

import "slices"

// recoverUndeliveredSteer rescues, at turn end, every steer message the turn
// never actually consumed, and enqueues it as the next message for the session.
// Native and CLI providers share one FIFO. A message arriving after the final
// model/tool boundary stays there until recovery. Closing admission and draining
// happen under one lock; explicit stop discards guidance instead of restarting.
//
// The messages go to the FRONT of the queue: the user typed them to redirect
// THIS turn, before anything they queued afterwards, so appending them to the
// tail would deliver their oldest intent last. They are pushed newest-first,
// because each push lands ahead of the previous one.
func (s *Server) recoverUndeliveredSteer(run *chatRun, wsID string, req chatReq) {
	undelivered := run.finishSteer()
	for _, u := range slices.Backward(undelivered) {
		s.enqueueMessageFront(wsID, chatReq{
			SessionID: req.SessionID,
			Message:   u,
			AgentIDs:  req.AgentIDs,
		})
	}
}
