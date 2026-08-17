package agent

import "context"

// ExternalActiveSessions reports session ids with a turn in flight that this
// runtime does NOT itself track — the api server's streamed (interactive) chat
// runs. The runtime only knows its own autonomous invokes (schedule wake,
// spawned sessions, inbox delivery, flow agent nodes), so without this half the
// picture is incomplete. Installed at startup by the api server, which owns the
// run registry; nil = no wiring (only the runtime's own sessions are seen).
type ExternalActiveSessions func() []string

// SetExternalActiveSessions wires the api server's chat-run registry into this
// runtime. The workspace manager calls it for every runtime (existing + later
// opened), mirroring SetAutonomousInteraction.
func (r *Runtime) SetExternalActiveSessions(fn ExternalActiveSessions) { r.extActive = fn }

// AgentBusy reports whether the agent has work in flight right now: a turn in
// any session it owns or takes part in. The second return is the id of the first
// live thing found, for the caller's message.
//
// This is the ONE implementation, deliberately: an agent can be deleted from two
// places — the HTTP endpoint and the self-management delete_agent tool — and a
// guard that only covers one of them is worse than none, because it reads as
// protection. Both go through here.
//
// Neither live registry is keyed by agent (both track sessions), so the active
// session ids are intersected with the agent's own sessions. Ownership is not
// enough: in a multi-agent thread the responder is often a participant rather
// than the owner.
func (r *Runtime) AgentBusy(ctx context.Context, agentID string) (bool, string) {
	if r == nil || agentID == "" {
		return false, ""
	}
	live := r.ActiveSessionIDs()
	if r.extActive != nil {
		live = append(live, r.extActive()...)
	}
	for _, sid := range live {
		sess, err := r.db.GetSession(ctx, sid)
		if err != nil {
			continue // vanished between the snapshot and the lookup
		}
		if sess.AgentID == agentID {
			return true, sess.ID
		}
		for _, p := range sess.Participants {
			if p == agentID {
				return true, sess.ID
			}
		}
	}
	return false, ""
}
