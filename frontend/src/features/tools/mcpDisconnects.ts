// Mid-session MCP disconnect notices for the Tools screen.
//
// The backend emits ws:mcp_status only for an UNEXPECTED connection death —
// idle reap, session close, config re-dial and pool shutdown are filtered out
// down in internal/mcp, so anything arriving here is a real failure worth
// showing. This module keeps the pure reducer side (what the UI should display
// given a stream of events) out of the React hook, so it can be unit-tested
// without a live stream.

import type { MCPStatusData } from '@/api/workspaceEvents'

// MCPDisconnectNotice is one server's most recent loss, as the UI renders it.
export interface MCPDisconnectNotice {
  server: string
  // scoped: the dead connection belonged to ONE (session, agent) pair. The
  // server itself may still be serving other sessions, so the UI must word this
  // differently from a shared-connection loss.
  scoped: boolean
  sessionId?: string
  error?: string
  pendingCalls: number
  at: number
}

// applyDisconnect folds one event into the notice list, newest first, keeping at
// most one entry per server: a server that dies repeatedly (a crash loop) is one
// problem, not a growing pile of cards.
export function applyDisconnect(
  current: MCPDisconnectNotice[],
  ev: MCPStatusData,
): MCPDisconnectNotice[] {
  if (ev.op !== 'disconnected' || !ev.server) return current
  const notice: MCPDisconnectNotice = {
    server: ev.server,
    scoped: ev.scoped,
    sessionId: ev.sessionId,
    error: ev.error,
    pendingCalls: ev.pendingCalls,
    at: ev.at,
  }
  return [notice, ...current.filter((n) => n.server !== ev.server)]
}

// dismissDisconnect drops one server's notice (the user acknowledged it, or a
// later pool snapshot showed the server live again).
export function dismissDisconnect(
  current: MCPDisconnectNotice[],
  server: string,
): MCPDisconnectNotice[] {
  return current.filter((n) => n.server !== server)
}

// clearRecovered drops notices for servers the pool now reports as having at
// least one live connection. The pool re-dials transparently on next use, so a
// stale "disconnected" card would otherwise outlive the outage it describes.
export function clearRecovered(
  current: MCPDisconnectNotice[],
  liveServers: readonly { server: string; live: number }[],
): MCPDisconnectNotice[] {
  const live = new Set(liveServers.filter((s) => s.live > 0).map((s) => s.server))
  return current.filter((n) => !live.has(n.server))
}
