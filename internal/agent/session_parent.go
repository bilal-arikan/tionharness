package agent

import (
	"context"
	"strings"
)

// SessionParentID returns the session a session hangs off — the coordinator that
// spawned a worker, the session a subagent was delegated from, the one a handoff
// continues — or "" for a top-level session that has no parent.
//
// It is the single source of truth for the process ledger's
// procwatch.Owner.ParentSessionID: the parentage lives on the session row
// (db.Session.ParentSessionID, written by SpawnSession and the subagent runner),
// not on anything the turn carries, so every stamp site has to read it from here.
//
// A lookup failure returns "" — the caller always needs an id back — but it is
// logged rather than swallowed: a session that HAS a parent silently losing it
// would ungroup a whole fan-out in the panel with no other trace.
func (r *Runtime) SessionParentID(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	s, err := r.db.GetSession(context.Background(), sessionID)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("process ledger: parent session lookup failed; the entry is not grouped under its coordinator",
				"session", sessionID, "error", err)
		}
		return ""
	}
	return strings.TrimSpace(s.ParentSessionID)
}
