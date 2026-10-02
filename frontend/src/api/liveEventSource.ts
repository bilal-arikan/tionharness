import { subscribeLiveChannel } from './liveConnection'
import { parseLiveFrame } from './liveFrames'

// EventSource-shaped adapter keeps global notification consumers unchanged,
// while the underlying connection also carries the workspace and chat feeds.
export class LiveEventSource {
  readyState: number = 0
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  private listeners = new Map<string, Set<(event: MessageEvent) => void>>()
  private unsubscribe: () => void

  constructor() {
    this.unsubscribe = subscribeLiveChannel(() => ({ key: 'global', scope: 'global' }), {
      onOpen: () => {
        this.readyState = 1
        this.onopen?.()
      },
      onClose: () => {
        if (this.readyState === 2) return
        this.readyState = 0
        this.onerror?.()
      },
      onFrame: (frame) => {
        const parsed = parseLiveFrame(frame)
        if (!parsed) return
        const event = new MessageEvent(parsed.event, { data: JSON.stringify(parsed.data) })
        this.listeners.get(parsed.event)?.forEach((cb) => cb(event))
      },
    })
  }

  addEventListener(name: string, listener: (event: MessageEvent) => void) {
    if (!this.listeners.has(name)) this.listeners.set(name, new Set())
    this.listeners.get(name)!.add(listener)
  }

  close() {
    this.readyState = 2
    this.unsubscribe()
    this.listeners.clear()
  }
}
