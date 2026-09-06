package api

// recoverUndeliveredSteer rescues, at turn end, every steer message the turn
// never actually consumed, and enqueues it as the next message for the session.
// Two sources, both empty on a normal turn:
//
//   - the steer CHANNEL, which the native tool loop drains before each provider
//     call. A message that arrives after the last drain point is still sitting
//     there when the turn returns — and on the tool-less path, where the turn
//     makes a single completion, that is every message sent mid-completion.
//   - the claude-cli pendingSteer stash, delivered at the next tool boundary as
//     the Interaction MCP permission tool's additionalContext, so it survives a
//     turn that called no tool at all.
//
// The messages go to the FRONT of the queue: the user typed them to redirect
// THIS turn, before anything they queued afterwards, so appending them to the
// tail would deliver their oldest intent last. They are pushed newest-first,
// because each push lands ahead of the previous one.
func (s *Server) recoverUndeliveredSteer(run *chatRun, wsID string, req chatReq) {
	undelivered := run.takeSteerQueue()
	if msg := run.takeSteer(); msg != "" {
		undelivered = append(undelivered, msg)
	}
	for i := len(undelivered) - 1; i >= 0; i-- {
		s.enqueueMessageFront(wsID, chatReq{
			SessionID: req.SessionID,
			Message:   undelivered[i],
			AgentIDs:  req.AgentIDs,
		})
	}
}
