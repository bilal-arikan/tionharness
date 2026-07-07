import { useCallback, useEffect, useRef, useState } from 'react'

export interface UseAsyncOptions {
  // When set, re-run `fn` on this interval (ms) after the initial load. Polling
  // is paused while the component is unmounted.
  pollMs?: number
  // When false, skip the initial (and polled) fetch entirely. Useful to defer a
  // load until some precondition holds. Defaults to true.
  enabled?: boolean
}

export interface UseAsyncState<T> {
  data: T | null
  loading: boolean
  error: string | null
  // Imperatively re-run the async function (e.g. a manual refresh button).
  refresh: () => void
}

// useAsync runs an async producer and tracks {data, loading, error}. It is
// unmount-safe: results (and errors) from an in-flight call are ignored once the
// component has unmounted, and stale results are ignored once `fn`/deps change.
// Optional polling re-runs `fn` on an interval.
//
// `fn` is captured per-render; pass the values it closes over via `deps` (same
// contract as useEffect's dependency array) so the effect re-subscribes when
// they change.
export function useAsync<T>(
  fn: () => Promise<T>,
  deps: React.DependencyList,
  opts: UseAsyncOptions = {},
): UseAsyncState<T> {
  const { pollMs, enabled = true } = opts
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Alive across the component's lifetime; flipped false on unmount so late
  // resolutions never call setState on a gone component.
  const aliveRef = useRef(true)
  useEffect(() => {
    aliveRef.current = true
    return () => {
      aliveRef.current = false
    }
  }, [])

  // Bump on each run so only the most recent call is allowed to commit — guards
  // against out-of-order resolutions when deps change mid-flight.
  const runIdRef = useRef(0)

  const run = useCallback(() => {
    const myId = ++runIdRef.current
    setLoading(true)
    setError(null)
    fn()
      .then((res) => {
        if (!aliveRef.current || runIdRef.current !== myId) return
        setData(res)
      })
      .catch((e) => {
        if (!aliveRef.current || runIdRef.current !== myId) return
        setError((e as Error).message)
      })
      .finally(() => {
        if (!aliveRef.current || runIdRef.current !== myId) return
        setLoading(false)
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  useEffect(() => {
    if (!enabled) return
    run()
    if (!pollMs) return
    const t = setInterval(run, pollMs)
    return () => clearInterval(t)
  }, [run, pollMs, enabled])

  return { data, loading, error, refresh: run }
}
