import { useCallback, useState } from 'react'
import { classifyWidth } from '@/shared/lib/viewport'

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

// A strip with no stored preference starts open where there is room for it and
// folded on a phone-width viewport, where three rows of chips would push the
// content they filter off the first screen.
function roomyViewport(): boolean {
  if (typeof window === 'undefined') return true
  return classifyWidth(window.innerWidth) !== 'narrow'
}

// useFilterDisclosure returns the persisted open flag for the strip `id` and a
// toggle that flips and stores it. `defaultOpen` pins the no-preference state;
// left out, it follows the viewport (see roomyViewport).
export function useFilterDisclosure(id: string, defaultOpen?: boolean): [boolean, () => void] {
  const [open, setOpen] = useState(() => readOpen(id, defaultOpen ?? roomyViewport()))
  const toggle = useCallback(() => {
    setOpen((prev) => {
      writeOpen(id, !prev)
      return !prev
    })
  }, [id])
  return [open, toggle]
}
