// useRefreshTrigger subscribes the calling component to a refresh-signal key.
// The hook returns the current tick; whenever the tick advances (App.tsx's
// central SSE handler calls bumpSignal on the matching key), the component
// re-renders and any useEffect with `tick` in its deps re-runs.
//
// Typical use: a panel that fetches its own data exposes a `reload` function
// and pairs it with the trigger in a single useEffect:
//
//   const tick = useRefreshTrigger('board')
//   useEffect(() => { reload() }, [tick, filter])
//
// The first render yields tick=0, so the effect also runs on mount and pulls
// the initial data. No SSE plumbing inside the panel — App.tsx is the single
// dispatcher, panels are dumb consumers.
import { useSyncExternalStore } from 'react'
import { getSnapshot, subscribeSignal } from '@/shared/lib/refreshSignals'

export function useRefreshTrigger(key: string): number {
  return useSyncExternalStore(
    (cb) => subscribeSignal(key, cb),
    () => getSnapshot(key),
    // SSR snapshot — the store is empty on the server so 0 is the safe default.
    () => 0,
  )
}
