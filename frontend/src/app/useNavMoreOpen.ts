import { useCallback, useEffect, useState } from 'react'
import { MORE_NAV } from './navItems'
import type { View } from './viewMetadata'

const MORE_OPEN_KEY = 'tionharness.navMoreOpen'

function readStored(): boolean {
  try {
    return localStorage.getItem(MORE_OPEN_KEY) === '1'
  } catch {
    return false
  }
}

// isMoreView reports whether a view lives in the rail's "More" group.
export function isMoreView(view: View): boolean {
  return MORE_NAV.some((item) => item.key === view)
}

// useNavMoreOpen owns the open state of the labelled rail's "More" group. The
// user's choice persists device-locally (like the rail's collapsed flag), and the
// group opens by itself whenever the active view moves into it, so the current
// destination is never hidden behind a closed disclosure on arrival.
export function useNavMoreOpen(view: View) {
  const [open, setOpen] = useState(() => readStored() || isMoreView(view))

  // Auto-open on entering the group (derived-state-during-render pattern, so no
  // effect-time setState cascade). Leaving the group keeps the user's state.
  const [seenView, setSeenView] = useState(view)
  if (seenView !== view) {
    setSeenView(view)
    if (isMoreView(view) && !open) setOpen(true)
  }

  useEffect(() => {
    try {
      localStorage.setItem(MORE_OPEN_KEY, open ? '1' : '0')
    } catch {
      /* storage unavailable — non-fatal, the group just won't persist */
    }
  }, [open])

  const toggle = useCallback(() => setOpen((prev) => !prev), [])
  return { open, toggle }
}
