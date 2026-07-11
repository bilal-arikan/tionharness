// Session event stream client — the frontend half of the server-authoritative
// per-session hub (internal/sessionhub, _Docs/58-QUEUE-SENKRON.md). Every window
// watching a session opens ONE of these and renders the transcript live from it,
// whoever started the turn. It replaces the old owner-streams-its-own-SSE +
// non-owner-polls-inflight split.
//
// Robustness (the session may be reached over a network — Doc 48 VPS client):
//   - cursor: tracks the last durable seq applied; reconnects carry ?since=<seq>
//     so the server gap-fills from its ring.
//   - epoch: the server's per-boot id; a mismatch (restart) triggers a reset.
//   - gap detection: a durable frame whose seq skips ahead (a dropped frame under
//     load) forces a reconnect from the last good seq to gap-fill.
//   - reset: when the cursor is unusable (epoch changed, or it fell out of the
//     server ring) the caller does a full resync (listMessages) via onReset.
//   - auto-reconnect: on stream end/error, retry with a bounded backoff.
import { getActiveWorkspace } from './client'

// A stable id for THIS browser window/tab, minted once. Used to tag outbound
// signals (typing) so the window can ignore its own echo on the shared hub.
export const windowClientId: string = (() => {
  const c = (globalThis as { crypto?: { randomUUID?: () => string } }).crypto
  if (c?.randomUUID) return c.randomUUID()
  return `win-${Date.now()}-${Math.random().toString(36).slice(2)}`
})()

// One hub event as delivered by the server. Payload is kind-specific JSON the
// caller narrows on `kind` (a TurnStep for "step", a Message for "reply", …).
export interface HubEvent {
  seq: number
  sessionId: string
  kind: string
  payload?: unknown
  time: number
}

export interface SessionStreamHandlers {
  // hello fires once per (re)connection with the server's current epoch + head.
  onHello?: (info: { epoch: string; head: number }) => void
  // reset fires when the cursor is unusable: the caller must resync from scratch
  // (listMessages) and then keep applying live events from `head`.
  onReset?: (info: { head: number }) => void
  // event fires for every durable or ephemeral hub event, in order.
  onEvent: (ev: HubEvent) => void
  // open/close are optional lifecycle hooks (e.g. presence / "reconnecting" UI).
  onOpen?: () => void
  onClose?: () => void
}

// Kinds mirror internal/sessionhub constants.
export const HubKind = {
  UserMessage: 'user_message',
  Step: 'step',
  Reply: 'reply',
  AgentStart: 'agent_start',
  InteractionOpen: 'interaction_open',
  InteractionResolved: 'interaction_resolved',
  SessionUpdate: 'session_update',
  TurnDone: 'turn_done',
  TurnError: 'turn_error',
  QueueUpdate: 'queue_update',
  Presence: 'presence',
  Delta: 'delta',
  ToolDelta: 'tool_delta',
  Typing: 'typing',
} as const

// subscribeSessionStream opens the stream and keeps it alive across reconnects.
// Returns an unsubscribe function that stops the loop and aborts the fetch.
export function subscribeSessionStream(
  sessionId: string,
  handlers: SessionStreamHandlers,
): () => void {
  let closed = false
  let ac: AbortController | null = null
  let since = 0 // last durable seq applied (the cursor)
  let epoch = '' // server epoch this cursor belongs to
  let backoff = 500

  const parseFrame = (frame: string): { event: string; data: unknown } | null => {
    let event = 'message'
    const dataLines: string[] = []
    for (const line of frame.split('\n')) {
      if (line.startsWith('event:')) event = line.slice(6).trim()
      else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
    }
    if (dataLines.length === 0) return null
    try {
      return { event, data: JSON.parse(dataLines.join('\n')) }
    } catch {
      return null
    }
  }

  // handle returns true to force an immediate reconnect (gap detected).
  const handle = (event: string, data: unknown): boolean => {
    switch (event) {
      case 'hello': {
        const h = data as { epoch: string; head: number }
        epoch = h.epoch
        handlers.onHello?.(h)
        return false
      }
      case 'reset': {
        const h = data as { head: number }
        since = h.head // trust the server's live edge; caller resyncs history
        handlers.onReset?.(h)
        return false
      }
      case 'hub': {
        const ev = data as HubEvent
        // Gap detection: a durable frame that skips ahead means we dropped one
        // under load. Reconnect from the last good seq so the ring gap-fills it.
        if (ev.seq > 0 && since > 0 && ev.seq > since + 1) {
          return true
        }
        handlers.onEvent(ev)
        if (ev.seq > since) since = ev.seq
        return false
      }
      default:
        return false
    }
  }

  const connect = async () => {
    while (!closed) {
      ac = new AbortController()
      try {
        const ws = getActiveWorkspace()
        const qs = new URLSearchParams()
        if (since > 0) qs.set('since', String(since))
        if (epoch) qs.set('epoch', epoch)
        if (ws) qs.set('ws', ws)
        const res = await fetch(`/api/sessions/${sessionId}/stream?${qs.toString()}`, {
          headers: ws ? { 'X-Workspace-Id': ws } : {},
          signal: ac.signal,
        })
        if (!res.ok || !res.body) {
          throw new Error(`stream HTTP ${res.status}`)
        }
        backoff = 500 // a successful open resets the backoff
        handlers.onOpen?.()
        const reader = res.body.getReader()
        const decoder = new TextDecoder()
        let buf = ''
        let forceReconnect = false
        for (;;) {
          const { done, value } = await reader.read()
          if (done) break
          buf += decoder.decode(value, { stream: true })
          let idx: number
          while ((idx = buf.indexOf('\n\n')) >= 0) {
            const frame = buf.slice(0, idx)
            buf = buf.slice(idx + 2)
            if (!frame.trim() || frame.startsWith(':')) continue // ping/comment
            const parsed = parseFrame(frame)
            if (parsed && handle(parsed.event, parsed.data)) {
              forceReconnect = true
              break
            }
          }
          if (forceReconnect) break
        }
        handlers.onClose?.()
        try { ac.abort() } catch { /* already aborting */ }
        if (forceReconnect) continue // immediate reconnect to gap-fill
      } catch {
        handlers.onClose?.()
        if (closed) break
      }
      if (closed) break
      // Bounded backoff before reconnecting on end/error.
      await new Promise((r) => setTimeout(r, backoff))
      backoff = Math.min(backoff * 2, 10_000)
    }
  }

  void connect()

  return () => {
    closed = true
    if (ac) {
      try { ac.abort() } catch { /* noop */ }
    }
  }
}
