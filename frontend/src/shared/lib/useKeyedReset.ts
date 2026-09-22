import { useState } from 'react'

// useKeyedReset runs `reset` during render whenever `key` changes (not on the
// first render), which is React's sanctioned way to adjust state when a prop or
// derived value changes: the setters called from `reset` schedule an immediate
// re-render BEFORE anything is committed, so the screen never paints the previous
// key's state. It replaces the `useEffect(() => { setX(initial) }, [key])` idiom,
// which painted the stale state first and then re-rendered (the pattern the
// react-hooks/set-state-in-effect rule flags).
//
// Only the calling component's own state may be set from `reset`; a parent's
// setter or a store write belongs in an effect or an event handler. Comparison
// is Object.is, so compose a string key when several inputs matter:
// useKeyedReset(`${a}|${b}`, …).
export function useKeyedReset<K>(key: K, reset: (key: K) => void): void {
  const [prev, setPrev] = useState(key)
  if (!Object.is(prev, key)) {
    setPrev(key)
    reset(key)
  }
}
