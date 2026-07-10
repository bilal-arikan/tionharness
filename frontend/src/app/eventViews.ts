// Maps a backend AppEvent `type` to the nav View whose badge it should light, so
// the generic "something changed" signal (SSE events) drives per-view unread dots
// uniformly. App-global types (settings/workspaces) return null — they refresh
// state but never badge a single view.
import type { View } from './NavRail'

export function viewForEventType(type: string): View | null {
  switch (type) {
    case 'chat':
      return 'chat'
    case 'flow':
      return 'flows'
    case 'task':
    case 'board':
      return 'board'
    case 'artifact':
      return 'artifacts'
    case 'schedule':
      return 'schedules'
    case 'spawned':
    case 'worker':
    case 'coordination':
      // Spawned / worker / coordination runs are sessions; their transcripts live
      // in the unified chat sidebar, so their unread badge lands on 'chat'.
      return 'chat'
    default:
      return null
  }
}
