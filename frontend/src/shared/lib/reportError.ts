// Frontend error bridge: forwards client-side crashes to the backend log stream
// (POST /api/logs) so a white-screen React crash or an unhandled promise
// rejection shows up in the in-app Logs screen instead of dying in the browser
// console where nobody is watching.

export interface ClientErrorReport {
  source: string
  message: string
  stack?: string
  level?: 'error' | 'warn' | 'info'
}

// Throttle: collapse identical messages within a short window so a render loop
// that throws every frame can't flood the ring buffer (or the network).
const recent = new Map<string, number>()
const THROTTLE_MS = 5000

function throttled(key: string): boolean {
  const now = Date.now()
  const last = recent.get(key)
  if (last !== undefined && now - last < THROTTLE_MS) return true
  recent.set(key, now)
  // Opportunistic cleanup so the map can't grow unbounded.
  if (recent.size > 100) {
    for (const [k, t] of recent) if (now - t > THROTTLE_MS) recent.delete(k)
  }
  return false
}

// reportClientError sends one report, best-effort. It never throws and never
// awaits meaningfully — a failure to report must not cascade into more errors.
export function reportClientError(rep: ClientErrorReport): void {
  try {
    const key = `${rep.source}:${rep.message}`
    if (throttled(key)) return
    const body = JSON.stringify({
      level: rep.level ?? 'error',
      source: rep.source,
      message: rep.message,
      stack: rep.stack ?? '',
      url: typeof location !== 'undefined' ? location.href : '',
    })
    // keepalive lets the report survive a page unload (e.g. a crash on navigation).
    void fetch('/api/logs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body,
      keepalive: true,
    }).catch(() => {})
  } catch {
    // Swallow — reporting must be invisible and side-effect-free on failure.
  }
}

// installGlobalErrorHandlers wires window-level catch-alls for errors that no
// component boundary sees: uncaught exceptions and unhandled promise rejections.
// Call once at startup.
export function installGlobalErrorHandlers(): void {
  if (typeof window === 'undefined') return

  window.addEventListener('error', (e: ErrorEvent) => {
    const err = e.error as Error | undefined
    reportClientError({
      source: 'window.onerror',
      message: err?.message || e.message || 'uncaught error',
      stack: err?.stack,
    })
  })

  window.addEventListener('unhandledrejection', (e: PromiseRejectionEvent) => {
    const reason = e.reason
    const message =
      reason instanceof Error
        ? reason.message
        : typeof reason === 'string'
          ? reason
          : 'unhandled promise rejection'
    reportClientError({
      source: 'unhandledrejection',
      message,
      stack: reason instanceof Error ? reason.stack : undefined,
    })
  })
}
