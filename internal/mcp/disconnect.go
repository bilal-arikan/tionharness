package mcp

import (
	"log/slog"
	"strings"
)

// Mid-session disconnect detection.
//
// The pool already re-dials a dead connection transparently on next use, but
// until something WATCHES for the death, nobody learns about it: the user sees
// tools silently vanish from a later turn and the model sees a tool it just used
// stop existing. StdioClient.failAll is the one place that knows a connection
// died, so the callback starts there and is surfaced here as a pool-level hook
// a higher layer (the agent runtime) turns into an event + an agent note.
//
// The hard requirement is NO false positives. Every clean teardown path in the
// pool — idle reap (reapScoped), CloseSession, config-change re-dial in ensure,
// Pool.Close — closes the client through Client.Close(), which sets closed=true
// BEFORE the read loop unwinds. failAll fires the callback only when that flag
// was still unset (client.go's wasClosed gate), so an intentional close is
// structurally incapable of producing a disconnect event. The tests in
// disconnect_test.go pin one clean path each.

// DisconnectEvent describes one MCP server connection that died unexpectedly.
//
// Scoped and shared connections are reported distinctly on purpose: a scoped
// slot belongs to a single (session, agent) pair, so its death must not be
// rendered as "the server is down" for the whole workspace while other sessions
// still hold live connections to the same server.
type DisconnectEvent struct {
	// Server is the sanitized server name (the pool key, not the display name).
	Server string `json:"server"`
	// Scoped is true for a per-(session,agent) connection, false for the shared
	// workspace-wide one.
	Scoped bool `json:"scoped"`
	// ScopeKey is "<sessionID>|<agentID>" for a scoped connection, "" for shared.
	ScopeKey string `json:"scopeKey"`
	// Error is why the read loop exited (often EOF when the process just quit).
	Error string `json:"error"`
	// PendingCalls is how many in-flight tool calls the death stranded. A
	// non-zero value means a turn was actively using the server as it went away.
	PendingCalls int `json:"pendingCalls"`
}

// SessionID returns the session part of a scoped connection's key, or "" for a
// shared connection. Lets a consumer route the notice to the affected session.
func (e DisconnectEvent) SessionID() string {
	if !e.Scoped {
		return ""
	}
	id, _, _ := strings.Cut(e.ScopeKey, "|")
	return id
}

// SetOnDisconnect registers a callback fired (in its own goroutine, from the
// dying client's read loop) whenever a pooled connection dies UNEXPECTEDLY.
// Clean shutdowns never reach it. Safe to leave unset.
func (p *Pool) SetOnDisconnect(fn func(DisconnectEvent)) {
	p.mu.Lock()
	p.onDisconnect = fn
	p.mu.Unlock()
}

// noteDisconnect is the per-client callback the pool installs in ensure. key is
// the pool-entry key, which carries both the server name and the scope.
func (p *Pool) noteDisconnect(key string, err error, pendingCalls int) {
	ev := DisconnectEvent{PendingCalls: pendingCalls}
	if err != nil {
		ev.Error = err.Error()
	}
	ev.Server, ev.ScopeKey, ev.Scoped = splitEntryKey(key)
	p.log(slog.LevelWarn, "mcp pool: connection lost",
		"server", ev.Server, "scoped", ev.Scoped, "pending", pendingCalls, "error", ev.Error)
	p.mu.Lock()
	fn := p.onDisconnect
	p.mu.Unlock()
	if fn != nil {
		fn(ev)
	}
}

// splitEntryKey decomposes a pool-entry key into its server name and scope. A
// key without scopeSep is a shared slot whose whole key is the server name.
func splitEntryKey(key string) (server, scopeKey string, scoped bool) {
	if scope, name, ok := strings.Cut(key, scopeSep); ok {
		return name, scope, true
	}
	return key, "", false
}
