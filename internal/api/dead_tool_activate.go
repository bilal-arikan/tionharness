package api

import "context"

// activateDeadTool is the api side of the agent runtime's dead-tool repair
// (internal/agent/deadtool.go). The model called a deferred extended tool before
// activating it, so the claude CLI rejected the call from its own registry and the
// gateway never saw it (WS20/SES79). The runtime spots that rejection on the CLI
// trace and calls in here to do server-side what activate_tools would have done.
//
// It reports whether bare is genuinely in this run's on-demand catalog. false —
// unknown run, unknown name, or a name the workspace/agent policy blocks — means
// the runtime must leave the original error alone: an invented tool name has to
// stay visibly wrong.
func (s *Server) activateDeadTool(_ context.Context, token, bare string) bool {
	if s.interBackend == nil {
		return false
	}
	b := s.interBackend
	run := b.runs.byToken(token)
	if run == nil {
		return false
	}
	if !b.extendedCandidates(run)[bare] {
		return false
	}
	// Already-active names return no additions; the repair still counts as done, so
	// the model gets the "re-issue the same call" instruction either way — a tool
	// active server-side but missing from the CLI's registry is exactly the state a
	// list_changed push fixes.
	added := b.activateExtended(token, []string{bare})
	if len(added) > 0 && b.srv != nil {
		// Wait for the CLI to re-fetch tools/list, same as the activate_tools path:
		// the model reads the appended instruction and re-calls immediately, so the
		// tool must already be in its registry by then.
		b.srv.PushToolsChangedAndWait(token, activateRelistTimeout)
	}
	return true
}
