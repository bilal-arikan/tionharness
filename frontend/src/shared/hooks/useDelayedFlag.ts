import { useEffect, useState } from 'react'

// useDelayedFlag turns `active` on only after it has stayed true for `delayMs`,
// and turns it off immediately when `active` goes false. Use it to gate loading
// skeletons: a local backend usually answers in well under the delay, so a fast
// load never flashes a placeholder, while a genuinely slow one still shows it.
export function useDelayedFlag(active: boolean, delayMs = 140): boolean {
  // Only ever set from the timer callback. The "off" transition is derived below
  // rather than written back into state, so a deactivation costs no extra render.
  const [elapsed, setElapsed] = useState(false)
  useEffect(() => {
    if (!active) return
    const handle = setTimeout(() => setElapsed(true), delayMs)
    return () => {
      clearTimeout(handle)
      setElapsed(false)
    }
  }, [active, delayMs])
  return active && elapsed
}
