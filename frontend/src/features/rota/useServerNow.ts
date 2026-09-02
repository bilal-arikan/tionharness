// useServerNow ticks a unix-seconds clock every `intervalMs` so live bars grow
// and the "now" line advances without a stream event. Uses the shared server
// clock when available (hub hello feeds it) so the axis matches event stamps.
import { useEffect, useState } from 'react'
import { serverNow } from '@/shared/lib/serverClock'

export function useServerNow(intervalMs: number): number {
  const [now, setNow] = useState(() => serverNow())
  useEffect(() => {
    const id = window.setInterval(() => setNow(serverNow()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs])
  return now
}
