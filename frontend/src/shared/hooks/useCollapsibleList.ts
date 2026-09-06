import { useCallback, useState } from 'react'
import { useViewport } from './useViewport'

// useCollapsibleList owns the open/closed state of a screen's left list column
// (sessions, roster, artifacts, insight sub-pages …). The column has two very
// different presentations, so the flag is tracked per tier:
//   • md+ (docked column): a PERSISTED collapse under `storageKey`, default OPEN.
//     Collapsing it swaps the column for a slim reopen rail
//     (CollapsibleListShell) so content gets the width.
//   • narrow (drawer): an ephemeral drawer flag, default CLOSED and never
//     persisted, so a phone always lands on the content rather than an open
//     drawer.
// The returned `open` / `toggle` / `setOpen` always address the presentation
// that is active right now, so callers wire one pair of props regardless of tier.
export function useCollapsibleList(storageKey: string): {
  open: boolean
  toggle: () => void
  setOpen: (v: boolean) => void
} {
  const mobile = useViewport().tier === 'narrow'
  const [dockedOpen, setDockedOpen] = useState(() => readDockedOpen(storageKey))
  const [drawerOpen, setDrawerOpen] = useState(false)

  const setOpen = useCallback(
    (v: boolean) => {
      if (mobile) {
        setDrawerOpen(v)
        return
      }
      setDockedOpen(v)
      writeDockedOpen(storageKey, v)
    },
    [mobile, storageKey],
  )
  const toggle = useCallback(() => {
    if (mobile) {
      setDrawerOpen((v) => !v)
      return
    }
    setDockedOpen((v) => {
      writeDockedOpen(storageKey, !v)
      return !v
    })
  }, [mobile, storageKey])

  return { open: mobile ? drawerOpen : dockedOpen, toggle, setOpen }
}

function readDockedOpen(key: string): boolean {
  try {
    // Absent = default open; only an explicit '0' collapses.
    return globalThis.localStorage.getItem(key) !== '0'
  } catch {
    return true
  }
}

function writeDockedOpen(key: string, open: boolean) {
  try {
    globalThis.localStorage.setItem(key, open ? '1' : '0')
  } catch {
    // Persistence is best-effort when storage is blocked by browser policy.
  }
}
