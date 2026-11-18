import { useCallback, useEffect, useRef, useState } from 'react'

// Backoff between automatic attempts; the last delay repeats for as long as the
// load keeps failing, so a backend that comes up later is still picked up.
const RETRY_DELAYS_MS = [1000, 2000, 4000, 8000, 15000]

// retryDelayMs is the wait before the next attempt after `failures` consecutive
// failed ones (1-based).
export function retryDelayMs(failures: number): number {
  const index = Math.min(Math.max(failures, 1), RETRY_DELAYS_MS.length) - 1
  return RETRY_DELAYS_MS[index]
}

export interface RetryingLoad {
  // True until the first attempt settles (either way).
  loading: boolean
  // Message of the last failed attempt; null once an attempt has succeeded.
  error: string | null
  // True while an attempt that follows a failure is in flight.
  retrying: boolean
  // Start the next attempt now instead of waiting for the backoff.
  retry: () => void
}

// useRetryingLoad runs `load` on mount and keeps retrying with backoff until it
// succeeds. `onLoaded` receives the value plus `recovered` — whether at least
// one attempt failed first — so the caller can tell a clean boot from a late one.
// Both callbacks are read through refs, so inline closures do not restart it.
export function useRetryingLoad<T>(
  load: () => Promise<T>,
  onLoaded: (value: T, recovered: boolean) => void,
): RetryingLoad {
  const [state, setState] = useState({ loading: true, error: null as string | null })
  const [retrying, setRetrying] = useState(false)
  const loadRef = useRef(load)
  const onLoadedRef = useRef(onLoaded)
  useEffect(() => {
    loadRef.current = load
    onLoadedRef.current = onLoaded
  })
  const runRef = useRef<(() => void) | null>(null)

  useEffect(() => {
    let alive = true
    let inFlight = false
    let failures = 0
    let timer: ReturnType<typeof setTimeout> | undefined
    const run = () => {
      if (!alive || inFlight) return
      clearTimeout(timer)
      inFlight = true
      if (failures > 0) setRetrying(true)
      loadRef
        .current()
        .then((value) => {
          if (!alive) return
          const recovered = failures > 0
          failures = 0
          setState({ loading: false, error: null })
          onLoadedRef.current(value, recovered)
        })
        .catch((e: unknown) => {
          if (!alive) return
          failures += 1
          setState({ loading: false, error: e instanceof Error ? e.message : String(e) })
          timer = setTimeout(run, retryDelayMs(failures))
        })
        .finally(() => {
          inFlight = false
          if (alive) setRetrying(false)
        })
    }
    runRef.current = run
    run()
    return () => {
      alive = false
      clearTimeout(timer)
    }
  }, [])

  const retry = useCallback(() => runRef.current?.(), [])
  return { ...state, retrying, retry }
}
