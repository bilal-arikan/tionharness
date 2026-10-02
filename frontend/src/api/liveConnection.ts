import { parseLiveFrame } from './liveFrames'

export interface LiveChannelSpec {
  key: string
  scope: 'global' | 'workspace' | 'session'
  workspaceId?: string
  sessionId?: string
  since?: number
  epoch?: string
}

interface LiveChannelHandlers {
  onFrame: (frame: string) => void
  onOpen?: () => void
  onClose?: () => void
}

interface Subscriber {
  spec: () => LiveChannelSpec
  handlers: LiveChannelHandlers
}

// A single transport per tab leaves HTTP/1.1 slots for ordinary API requests.
// All scopes retain independent cursors, and background prompts remain live.
const subscribers = new Set<Subscriber>()
const statusListeners = new Set<() => void>()
export type LiveConnectionStatus = 'connecting' | 'connected' | 'reconnecting'
let status: LiveConnectionStatus = 'connecting'
let connectedOnce = false
let controller: AbortController | null = null
let timer: ReturnType<typeof setTimeout> | null = null
let revision = 0
let backoff = 500

function setStatus(next: LiveConnectionStatus) {
  if (next === status) return
  status = next
  statusListeners.forEach((cb) => cb())
}

export function getLiveConnectionStatus(): LiveConnectionStatus {
  return status
}

export function subscribeLiveConnectionStatus(cb: () => void): () => void {
  statusListeners.add(cb)
  return () => {
    statusListeners.delete(cb)
  }
}

function stopAttempt() {
  revision++
  if (timer !== null) clearTimeout(timer)
  timer = null
  controller?.abort()
  controller = null
}

function schedule(delay: number) {
  if (timer !== null) clearTimeout(timer)
  timer = setTimeout(() => {
    timer = null
    void connect()
  }, delay)
}

export function reconnectLiveConnection() {
  stopAttempt()
  backoff = 500
  if (subscribers.size) schedule(0)
}

export function subscribeLiveChannel(
  spec: () => LiveChannelSpec,
  handlers: LiveChannelHandlers,
): () => void {
  const subscriber = { spec, handlers }
  subscribers.add(subscriber)
  // Batch mount/unmount effects so changing a session opens one replacement,
  // rather than briefly holding both old and new persistent connections.
  reconnectLiveConnection()
  let disposed = false
  return () => {
    if (disposed) return
    disposed = true
    subscribers.delete(subscriber)
    handlers.onClose?.()
    reconnectLiveConnection()
  }
}

async function connect() {
  if (!subscribers.size) return
  const attempt = ++revision
  const ac = new AbortController()
  // Bound header waits and silent streams. The server pings every 25 seconds;
  // three missed pings indicate a dead proxy/socket rather than an idle chat.
  let watchdog = setTimeout(() => ac.abort(), 30_000)
  const noteTraffic = () => {
    clearTimeout(watchdog)
    watchdog = setTimeout(() => ac.abort(), 90_000)
  }
  controller = ac
  const snapshot = [...subscribers]
  const specs = [
    ...new Map(
      snapshot.map((s) => {
        const spec = s.spec()
        return [spec.key, spec] as const
      }),
    ).values(),
  ]
  const query = new URLSearchParams({ channels: JSON.stringify(specs) })
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined
  setStatus(connectedOnce ? 'reconnecting' : 'connecting')
  try {
    const res = await fetch(`/api/live/stream?${query}`, { signal: ac.signal, cache: 'no-store' })
    if (attempt !== revision) return
    if (!res.ok || !res.body) throw new Error(`live stream HTTP ${res.status}`)
    noteTraffic()
    connectedOnce = true
    backoff = 500
    setStatus('connected')
    snapshot.forEach((s) => {
      if (subscribers.has(s)) s.handlers.onOpen?.()
    })
    reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    for (;;) {
      const { done, value } = await reader.read()
      if (done || attempt !== revision) break
      noteTraffic()
      buffer += decoder.decode(value, { stream: true })
      let end: number
      while ((end = buffer.indexOf('\n\n')) >= 0) {
        const frame = parseLiveFrame(buffer.slice(0, end))
        buffer = buffer.slice(end + 2)
        if (frame?.event !== 'live') continue
        const payload = frame.data as { channel?: string; frame?: string }
        if (typeof payload?.channel !== 'string' || typeof payload.frame !== 'string') continue
        for (const s of snapshot) {
          if (attempt !== revision) break
          if (subscribers.has(s) && s.spec().key === payload.channel)
            s.handlers.onFrame(payload.frame)
        }
      }
    }
  } catch {
    // A disconnect belongs to this transport, never to an unhandled promise.
  } finally {
    clearTimeout(watchdog)
    ac.abort()
    try {
      await reader?.cancel()
    } catch {
      /* already disconnected */
    }
    reader?.releaseLock()
    if (attempt === revision) {
      controller = null
      setStatus(connectedOnce ? 'reconnecting' : 'connecting')
      snapshot.forEach((s) => {
        if (subscribers.has(s)) s.handlers.onClose?.()
      })
      if (subscribers.size) {
        schedule(backoff)
        backoff = Math.min(backoff * 2, 10_000)
      }
    }
  }
}
