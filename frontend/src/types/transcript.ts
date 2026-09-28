import type { Message, TodoItem } from './message'

export interface TranscriptSummary {
  lastMessageAt: number
  coldTurns: number
  todo: { todos: TodoItem[]; occurrenceId: string } | null
}

export interface MessagePage {
  items: Message[]
  offset: number
  total: number
  hasMore: boolean
  hasNewer: boolean
  summary?: TranscriptSummary
  participants?: string[]
}
