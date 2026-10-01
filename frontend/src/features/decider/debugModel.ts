import type { DeciderDebugEvent } from '@/types/decider'

export interface DebugTrace {
  id: string
  at: number
  authority: string
  events: DeciderDebugEvent[]
  outcome?: DeciderDebugEvent
  completed?: DeciderDebugEvent
}

// Input is newest first. Keep append order within equal timestamps, which is
// meaningful for a transport attempt followed by its enclosing model result.
export function groupDebugEvents(events: DeciderDebugEvent[]): DebugTrace[] {
  const groups = new Map<string, DebugTrace>()
  for (const event of [...events].reverse()) {
    let group = groups.get(event.traceId)
    if (!group) {
      group = { id: event.traceId, at: event.at, authority: event.authority, events: [] }
      groups.set(event.traceId, group)
    }
    group.at = Math.max(group.at, event.at)
    group.events.push(event)
    if (event.stage === 'outcome' && event.role !== 'challenger') group.outcome = event
    if (event.stage === 'completed') group.completed = event
  }
  return [...groups.values()].sort((a, b) => b.at - a.at)
}
