import { useEffect, useRef, useState } from 'react'

// useThrottledValue follows `value` at most once per `intervalMs`, always
// landing on the latest value (trailing edge). Unlike useDebouncedValue it never
// starves under a steady stream of changes: a burst of updates still yields one
// refresh per interval, so live views keep moving while the burst lasts.
export function useThrottledValue<T>(value: T, intervalMs: number): T {
  const [throttled, setThrottled] = useState(value)
  const lastEmit = useRef(0)
  useEffect(() => {
    const wait = Math.max(0, lastEmit.current + intervalMs - Date.now())
    const t = setTimeout(() => {
      lastEmit.current = Date.now()
      setThrottled(value)
    }, wait)
    return () => clearTimeout(t)
  }, [value, intervalMs])
  return throttled
}
