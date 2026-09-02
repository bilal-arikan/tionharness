// Maps a backend AppEvent `type` to the nav View whose badge it should light, so
// the generic "something changed" signal (SSE events) drives per-view unread dots
// uniformly. App-global types (settings/workspaces) return null — they refresh
// state but never badge a single view.
//
// Toastable events derive their mapping from the single notify-type registry
// (notifyTypes.ts), so a view assignment and its Settings toggle cannot drift.
// `skills` is deliberately a non-toast control event, handled here explicitly.
// `task` is a legacy alias for `board` (handled inside viewIdForType);
// spawned/worker runs are sessions, so their badge lands on 'chat'.
import type { View } from './NavRail'
import { viewIdForType } from '@/shared/lib/notifyTypes'

export function viewForEventType(type: string): View | null {
  if (type === 'skills') return 'skills'
  return viewIdForType(type) as View | null
}
