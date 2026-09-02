// Shared SSE loop behind the hub streams (per-session and per-workspace). Both
// endpoints speak the same wire contract — hello/reset/hub frames, a `since`
// cursor, a boot `epoch` — so the cursor tracking, gap detection, reset
// handling, reconnect backoff and server-clock calibration live here once.
// See sessionStream.ts / workspaceStream.ts for the typed entry points and
// _Docs/58-QUEUE-SENKRON.md for the protocol's reasoning.
import { getActiveWorkspace } from './client'
import { noteServerTime } from '@/shared/lib/serverClock'
import { SESSION_STREAM_CLOSED_MESSAGE } from '@/shared/lib/expectedAbort'

// One hub event as delivered by the server. Payload is kind-specific JSON the
// caller narrows on `kind`. sessionId is empty on workspace-stream events.
export interface HubEvent {
  seq: number
  sessionId: string
  kind: string
  payload?: unknown
  time: number
}

export interface HubStreamHandlers {
  // hello fires once per (re)connection with the server's current epoch + head
  // (+ `now`, the server's wall clock, already fed to the shared server clock).
  onHello?: (info: { epoch: string; head: number; now?: number }) => void
  // reset fires when the cursor is unusable: the caller must resync from scratch
  // and then keep applying live events from `head`.
  onReset?: (info: { head: number }) => void
  // event fires for every durable or ephemeral hub event, in order.
  onEvent: (ev: HubEvent) => void
  // open/close are optional lifecycle hooks (e.g. presence / "reconnecting" UI).
  onOpen?: () => void
  onClose?: () => void
}

function abortHubStream(controller: AbortController): void {
  controller.abort(new DOMException(SESSION_STREAM_CLOSED_MESSAGE, 'AbortError'))
}

// subscribeHubStream opens the stream at `path` (an /api route without query)
// and keeps it alive across reconnects. Returns an unsubscribe function that
// stops the loop and aborts the fetch.
export function subscribeHubStream(path: string, handlers: HubStreamHandlers): () => void {
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
        const h = data as { epoch: string; head: number; now?: number }
        epoch = h.epoch
        // Calibrate the shared server clock as early as possible: elapsed-time
        // counters must not tick against a skewed browser clock while waiting
        // for the first hub frame.
        noteServerTime(h.now ?? 0)
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
        const res = await fetch(`${path}?${qs.toString()}`, {
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
        try {
          abortHubStream(ac)
        } catch {
          /* already aborting */
        }
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
      try {
        abortHubStream(ac)
      } catch {
        /* noop */
      }
    }
  }
}
