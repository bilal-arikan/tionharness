package api

// scope.go holds the workspace-scoping primitive shared by every SERVER-WIDE,
// per-session structure in this package (the inbox queues, the interaction
// registry, the permission grants, the chat-run session lookups).
//
// Why it exists: session ids are allocated by each workspace's own file store
// (counters.json), so "SES1" is the first session of EVERY workspace — the ids
// are unique within a store, never across stores. Anything the server keeps in
// memory for all workspaces at once must therefore be keyed by the pair, or two
// unrelated sessions silently share one entry. That is what leaked a running
// conversation into a freshly-created workspace: same id, one shared hub/queue.
//
// The scope is server-side only; ids on the wire (hub events, URLs) stay bare —
// the workspace comes from the request's X-Workspace-Id, as everywhere else.

// scopeKey pairs a workspace id with a session id. NUL separates them because
// neither id can contain it, so no pair can alias another.
func scopeKey(wsID, sessionID string) string { return wsID + "\x00" + sessionID }
