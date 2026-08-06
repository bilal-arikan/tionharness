import { useEffect, useState } from 'react'

// useDebouncedValue returns `value` after it has been stable for `delayMs`.
// Used to turn a fast-typing search box into a single server request per pause
// (e.g. the Artifacts panel's server-side title search).
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delayMs)
    return () => clearTimeout(t)
  }, [value, delayMs])
  return debounced
}
