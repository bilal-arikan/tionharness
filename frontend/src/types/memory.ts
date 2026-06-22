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

// CoreBlock is one named MemGPT-style core-memory block: its definition (label,
// description, charLimit, readOnly) plus current content. Default blocks are
// persona + human; an agent may define more.
export interface CoreBlock {
  label: string
  description: string
  content: string
  charLimit: number
  readOnly: boolean
  order: number
}
