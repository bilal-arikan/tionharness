package db

import "strings"

// legacyInboxKind is the session kind peer messages used to land in, before
// TSK507 moved them into the recipient's ordinary chat thread. Nothing writes it
// any more; it only ever appears in stores written by an older build.
const legacyInboxKind = "inbox"

// peerThreadKind is the kind a peer-message thread carries now: the ordinary,
// writable chat kind. Mirrors internal/agent.inboxSessionKind — the two must
// agree, or a migrated session is not the one the runtime looks up.
const peerThreadKind = "chat"

// PeerThreadSourceID builds the SourceID that identifies one agent's standing
// peer-message thread among its other "chat" sessions.
//
// It lives here, next to the migration, because two places must agree on it or a
// migrated store silently grows a second thread: the runtime that looks the
// thread up on delivery (internal/agent.inboxSessionSource) and this migration,
// which stamps it onto the sessions it converts.
func PeerThreadSourceID(agentID string) string {
	return "agent-messages:" + agentID
}

// migrateLegacyInboxSessions converts pre-TSK507 "inbox" sessions into ordinary
// writable "chat" sessions, so transcripts written by an older build stop being
// read-only and become the same standing peer thread new deliveries append to.
//
// Each converted session is stamped with PeerThreadSourceID(agentID), which is
// what makes it the thread GetOrCreateSourceSession finds — without it the next
// delivery would not recognise the migrated session and would open a SECOND
// thread beside it, splitting the agent's message history in two.
//
// Runs once at Open, single-threaded, after sessions are loaded. Idempotent: a
// session already carrying the chat kind is not matched, so a re-run is a no-op.
//
// Best-effort per session, deliberately: a store may hold a session whose header
// cannot be rewritten (permissions, a file another process holds open). Failing
// the whole boot over one unconvertible transcript would take the workspace down
// to fix a display concern, so the error is counted and reported by the caller
// while every other session still migrates. It returns the counts rather than
// logging them itself, because the store has no logger at this point in boot.
func (d *DB) migrateLegacyInboxSessions() (converted, failed int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Snapshot the ids first: persistSessionLocked writes back into d.sessions,
	// and mutating a map while ranging over it is undefined.
	ids := make([]string, 0)
	for id, s := range d.sessions {
		if s.Kind == legacyInboxKind {
			ids = append(ids, id)
		}
	}

	for _, id := range ids {
		s := d.sessions[id]
		s.Kind = peerThreadKind
		// Only fill an EMPTY SourceID. A legacy inbox session has none, but if a
		// store somehow carries one it identifies that session to other lookups,
		// and overwriting it would repoint them at this thread.
		if strings.TrimSpace(s.SourceID) == "" {
			s.SourceID = PeerThreadSourceID(s.AgentID)
		}
		if err := d.persistSessionLocked(s); err != nil {
			failed++
			continue
		}
		converted++
	}
	return converted, failed
}
