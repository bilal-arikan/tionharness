package db

import "strings"

// Session identity for display.
//
// A chat, spawn or worker session names its owner in AgentID. A DELEGATED run
// (run_subagent) does not: it is created against a target, so its identity lives
// in TargetAgentID (an existing agent) or TargetProfile (a built-in profile, run
// through an ephemeral clone of the caller that is never persisted and so has no
// id at all). Any consumer that read AgentID alone rendered those rows anonymous.
//
// New delegated rows carry AgentID as well, so these helpers matter mostly for
// rows written before that — and for profile targets, which have no agent id by
// design and are therefore named rather than resolved.

// OwnerAgentID returns the agent id to resolve a name and avatar from: the
// session's own agent, else its delegation target. Empty when the session names
// no agent (a profile subagent, whose label comes from OwnerProfileLabel).
func (s Session) OwnerAgentID() string {
	if id := strings.TrimSpace(s.AgentID); id != "" {
		return id
	}
	return strings.TrimSpace(s.TargetAgentID)
}

// OwnerProfileLabel names a delegated run against a built-in profile, e.g.
// "subagent:coder". Empty for every session that has a real agent id.
func (s Session) OwnerProfileLabel() string {
	if s.OwnerAgentID() != "" {
		return ""
	}
	p := strings.TrimSpace(s.TargetProfile)
	if p == "" {
		return ""
	}
	return "subagent:" + p
}
