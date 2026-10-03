import { useEffect, useRef } from 'react'
import { createPollingSubscription } from './pollingSubscription'

// useVisiblePoll runs `fn` on an interval, but only while the document is
// visible — and fires one catch-up run when it becomes visible again.
//
// It is the setInterval counterpart of useAsync's `pauseWhenHidden` option, for
// panels that manage their own fetch state and so cannot use useAsync. Both
// exist for the same reason: with several windows open on one backend, the
// hidden ones were polling exactly as hard as the focused one, for indicators
// nobody could see.
//
// `fn` is captured per-render and read through a ref, so a changing callback
// identity never restarts the interval — pass real dependencies via `deps`.
// Passing `enabled: false` stops polling entirely (e.g. a run that reached a
// terminal state has nothing left to poll for).
export function useVisiblePoll(
  fn: () => void,
  intervalMs: number,
  deps: React.DependencyList = [],
  enabled = true,
): void {
  const fnRef = useRef(fn)
  useEffect(() => {
    fnRef.current = fn
  })

  useEffect(() => {
    if (!enabled) return
    return createPollingSubscription(() => fnRef.current(), intervalMs)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [intervalMs, enabled, ...deps])
}
