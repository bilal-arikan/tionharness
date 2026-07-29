import { useCallback, useLayoutEffect, useRef } from 'react'

// useStableCallback returns a callback whose IDENTITY never changes while it
// still forwards to the latest function it was given.
//
// It exists so React.memo on a transcript row actually holds: a memoized row
// re-renders whenever any prop changes identity, and a parent that declares its
// handlers inline (`onClick={() => …}`) hands out a fresh function every render
// — which defeats the memo entirely. Wrapping the handlers at the list boundary
// makes the memo independent of how the parent happens to declare them.
//
// An absent handler stays absent: callers test `!!onRetry` to decide whether to
// render the affordance at all, so this must not fabricate a no-op function.
export function useStableCallback<A extends unknown[], R>(
  fn: ((...args: A) => R) | undefined,
): ((...args: A) => R) | undefined {
  const ref = useRef(fn)
  // useLayoutEffect (not useEffect): a child may fire the callback from its own
  // layout effect, before a passive effect would have refreshed the ref.
  useLayoutEffect(() => {
    ref.current = fn
  })
  // Deliberately unguarded: if the handler disappeared between the render that
  // exposed it and the call, that is a real bug in the caller and should throw
  // rather than silently do nothing.
  const stable = useCallback((...args: A) => ref.current!(...args), [])
  return fn ? stable : undefined
}
