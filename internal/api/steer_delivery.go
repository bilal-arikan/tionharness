package api

// Shared plumbing for every caller that hands live guidance to a running turn:
// the session-scoped control endpoint (handleSessionControl) and the queue
// conversion endpoint (handleSteerQueued). Keeping the "which run owns the turn"
// and "which channel carries a steer" rules in ONE place means a new entry point
// cannot quietly grow its own, subtly different, notion of deliverability.

// steerOutcome reports what happened when a steer was handed to a run. There is
// no "silently dropped" value on purpose: every caller must answer the client
// with one of these.
type steerOutcome int

const (
	// steerDelivered: the guidance is on the run's steer channel, which the native
	// tool loop drains before its next provider call.
	steerDelivered steerOutcome = iota
	// steerStashed: the guidance is stashed on a claude-cli run, to be delivered at
	// the next tool boundary. Accepted, but not yet in front of the model — kept
	// distinct from steerDelivered so callers can report it as such.
	steerStashed
	// steerUnsupported: this turn has no boundary a steer could ride, so nothing
	// was handed over and the caller must say so instead of pretending it landed.
	steerUnsupported
	// steerBufferFull: the turn is not consuming guidance (buffer full) — the
	// message was NOT taken and the caller must report the failure.
	steerBufferFull
)

// steerTargetRun resolves the run that OWNS the session's in-flight turn — the
// only run a steer may target, since a superseded run's output is fenced out of
// the transcript and guidance sent to it could never surface. Returns nil plus a
// client-facing reason when there is no such run.
func (s *Server) steerTargetRun(wsID, sessionID string) (*chatRun, string) {
	info, live := s.runs.sessionRunInfo(wsID, sessionID)
	if !live {
		// An autonomous turn has no steer channel, so there is nothing to forward to.
		return nil, "no in-flight turn for this session"
	}
	run := s.runs.get(info.RunID)
	if run == nil {
		return nil, "run already finished"
	}
	return run, ""
}

// deliverSteer hands text to whichever mid-turn channel the run's provider has.
//
// Native providers drain the steer CHANNEL between tool-loop iterations (see
// agent/steer.go drainSteer). CLI providers (claude-cli, codex-cli) run their own
// subprocess loop with no such drain point, so instead the guidance is stashed on
// the run; on claude-cli it is delivered at the next tool boundary as the
// Interaction MCP permission tool's additionalContext (see callPermission). Only
// claude-cli in "ask" mode actually has such a boundary (see steerableForTurn) —
// every other CLI turn reports steerUnsupported rather than accepting a message
// that could only resurface at turn end.
//
// Never blocks: it runs on an HTTP handler goroutine (and, for the queue
// conversion, under the inbox lock) while the turn may be stalled in a long
// provider call.
func deliverSteer(run *chatRun, text string) steerOutcome {
	if provider := run.providerOf(); provider == "claude-cli" || provider == "codex-cli" {
		if !run.steerableFor() {
			return steerUnsupported
		}
		run.setSteer(text)
		return steerStashed
	}
	if !run.trySteer(text) {
		return steerBufferFull
	}
	return steerDelivered
}
