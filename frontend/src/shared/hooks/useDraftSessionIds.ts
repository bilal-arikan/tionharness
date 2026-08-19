import { useSyncExternalStore } from 'react'
import { draftSessionIds, subscribeSessionDrafts } from '@/shared/lib/sessionDrafts'

// Cached snapshot: useSyncExternalStore compares snapshots by identity, so a fresh
// Set on every read would loop forever. The cache is invalidated only when the
// store notifies, and the identity of an unchanged set is preserved by comparing
// the ids themselves (a keystroke inside an existing draft must not re-render).
let snapshot: Set<string> = new Set()
let primed = false

function sameIds(a: Set<string>, b: Set<string>): boolean {
  if (a.size !== b.size) return false
  for (const id of a) if (!b.has(id)) return false
  return true
}

function getSnapshot(): Set<string> {
  const next = draftSessionIds()
  if (primed && sameIds(snapshot, next)) return snapshot
  primed = true
  snapshot = next
  return snapshot
}

function subscribe(onChange: () => void): () => void {
  return subscribeSessionDrafts(onChange)
}

// useDraftSessionIds gives the ids of sessions holding an unsent composer draft in
// the active workspace, and re-renders when that set changes.
export function useDraftSessionIds(): Set<string> {
  return useSyncExternalStore(subscribe, getSnapshot, () => snapshot)
}
