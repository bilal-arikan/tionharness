// Agent memory entries (documents/journal/reflection) and recall previews.

export type MemoryKind = 'document' | 'journal' | 'reflection'

export interface Memory {
  id: string
  agentId: string
  kind: MemoryKind
  content: string
  createdAt: number
}

export interface RecallHit {
  id: string
  kind: MemoryKind
  content: string
  score: number
}
