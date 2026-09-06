// A tiny module-level store of nav Views that currently have UNSAVED local edits
// (the "dirty" signal). Editor screens register themselves via useRegisterDirty;
// the nav rail + workspace label subscribe via useDirtyViews. A module store (vs
// React context) avoids provider plumbing and works across the whole tree.
import { useEffect } from 'react'
import { useSyncExternalStore } from 'react'
import type { View } from '@/app/NavRail'

const dirty = new Set<View>()
const listeners = new Set<() => void>()
// Cached immutable snapshot so useSyncExternalStore sees a stable identity
// between changes (required — returning a fresh Set each call loops forever).
let snapshot: ReadonlySet<View> = new Set()

function refresh() {
  snapshot = new Set(dirty)
  listeners.forEach((l) => l())
}

// setViewDirty marks/clears a view's unsaved-edits flag. No-op when unchanged.
function setViewDirty(view: View, isDirty: boolean) {
  const had = dirty.has(view)
  if (isDirty && !had) {
    dirty.add(view)
    refresh()
  } else if (!isDirty && had) {
    dirty.delete(view)
    refresh()
  }
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb)
  return () => {
    listeners.delete(cb)
  }
}

function getSnapshot(): ReadonlySet<View> {
  return snapshot
}

// useDirtyViews returns the live set of views with unsaved edits.
export function useDirtyViews(): ReadonlySet<View> {
  return useSyncExternalStore(subscribe, getSnapshot)
}

// useRegisterDirty marks `view` dirty while `isDirty` holds, clearing it on
// change and on unmount (so leaving an editor never leaves a stale amber dot).
// `view` may be undefined so a shared editor (e.g. the agent form reused inside
// a modal) can opt out of registering — it then just no-ops.
export function useRegisterDirty(view: View | undefined, isDirty: boolean) {
  useEffect(() => {
    if (!view) return
    setViewDirty(view, isDirty)
    return () => setViewDirty(view, false)
  }, [view, isDirty])
}
