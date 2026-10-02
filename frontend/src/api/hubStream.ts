// Shared SSE loop behind the hub streams (per-session and per-workspace). Both
// endpoints speak the same wire contract — hello/reset/hub frames, a `since`
// cursor, a boot `epoch` — so the cursor tracking, gap detection, reset
// handling, reconnect backoff and server-clock calibration live here once.
// See sessionStream.ts / workspaceStream.ts for the typed entry points and
// _Docs/58-QUEUE-SENKRON.md for the protocol's reasoning.
import { getActiveWorkspace } from './client'
import { noteServerTime } from '@/shared/lib/serverClock'
import { parseLiveFrame } from './liveFrames'
import { reconnectLiveConnection, subscribeLiveChannel } from './liveConnection'

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

// subscribeHubStream opens the stream at `path` (an /api route without query)
// and keeps it alive across reconnects. Returns an unsubscribe function that
// stops the loop and aborts the fetch.
export function subscribeHubStream(path: string, handlers: HubStreamHandlers): () => void {
  const workspaceId = getActiveWorkspace() ?? ''
  let since = 0 // last durable seq applied (the cursor)
  let epoch = '' // server epoch this cursor belongs to

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
        // Subscribe+replay can overlap a live publish. Each durable fact is
        // applied once, including when another channel changes the transport.
        if (ev.seq > 0 && ev.seq <= since) return false
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

  const sessionId = path.match(/^\/api\/sessions\/([^/]+)\/stream$/)?.[1]
  return subscribeLiveChannel(
    () => ({
      key: `${workspaceId}:${path}`,
      scope: sessionId ? 'session' : 'workspace',
      workspaceId,
      sessionId,
      since,
      epoch,
    }),
    {
      onOpen: handlers.onOpen,
      onClose: handlers.onClose,
      onFrame: (raw) => {
        const parsed = parseLiveFrame(raw)
        if (parsed && handle(parsed.event, parsed.data)) reconnectLiveConnection()
      },
    },
  )
}
