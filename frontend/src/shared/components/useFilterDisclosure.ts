import { useCallback, useState } from 'react'

// Filter strips (session chips, Rota lanes, Explorer layers, tool tiers, note
// kinds, log levels) fold behind one toggle so a long chip row can be put away.
// The open state is remembered per strip; a folded strip still shows how many
// filters are narrowing the list, so a hidden filter never silently hides rows.
const KEY_PREFIX = 'tionharness.filtersOpen.'

function readOpen(id: string, fallback: boolean): boolean {
  try {
    const raw = globalThis.localStorage?.getItem(KEY_PREFIX + id)
    return raw == null ? fallback : raw === '1'
  } catch {
    return fallback
  }
}

function writeOpen(id: string, open: boolean): void {
  try {
    globalThis.localStorage?.setItem(KEY_PREFIX + id, open ? '1' : '0')
  } catch {
    // Persistence is best-effort when storage is blocked by browser policy.
  }
}

// useFilterDisclosure returns the persisted open flag for the strip `id` and a
// toggle that flips and stores it.
export function useFilterDisclosure(id: string, defaultOpen = true): [boolean, () => void] {
  const [open, setOpen] = useState(() => readOpen(id, defaultOpen))
  const toggle = useCallback(() => {
    setOpen((prev) => {
      writeOpen(id, !prev)
      return !prev
    })
  }, [id])
  return [open, toggle]
}
