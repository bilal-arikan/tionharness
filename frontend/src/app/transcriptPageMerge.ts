import type { Message } from '@/types'

// HTTP is a snapshot taken before its response arrives. Keep hub frames and
// optimistic edits that landed since that request, including an unfinished
// ghost; never resurrect a message removed while the snapshot was in flight.
export function mergeLiveTranscriptPage(
  snapshot: Message[],
  before: Message[],
  current: Message[],
): Message[] {
  const previous = new Map(before.map((m) => [m.id, m]))
  const now = new Map(current.map((m) => [m.id, m]))
  const result = snapshot.filter((m) => !previous.has(m.id) || now.has(m.id))
  for (const message of current) {
    if (!message.id.startsWith('live-') && previous.get(message.id) === message) continue
    const index = result.findIndex((m) => m.id === message.id)
    if (index >= 0) result[index] = message
    else result.push(message)
  }
  return result
}
