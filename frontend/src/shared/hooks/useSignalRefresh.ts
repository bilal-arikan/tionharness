import { useEffect, useRef } from 'react'

// Initial and workspace loads belong to the data hook. Only a new signal in
// the same scope should request another load, even when ticks predate mount.
export function useSignalRefresh(
  scope: string | null,
  signal: string | number,
  refresh: () => void,
) {
  const previous = useRef({ scope, signal })
  useEffect(() => {
    const before = previous.current
    previous.current = { scope, signal }
    if (before.scope === scope && before.signal !== signal) refresh()
  }, [scope, signal, refresh])
}
