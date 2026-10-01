import { req } from './client'

export interface SessionDecisionItem {
  key: string
  kind: string
  label: string
  action: string
  strength?: number
}

export interface SessionDecisionEntry {
  id: string
  at: number
  authority: string
  mode: 'off' | 'shadow' | 'on'
  status: 'applied' | 'observed' | 'fallback'
  traceId?: string
  recommended?: string
  applied?: string
  baseline?: string
  items: SessionDecisionItem[]
  error?: string
}

export interface SessionDecisionMemory {
  key: string
  label: string
  sourceId?: string
  text?: string
  pinned: boolean
  mandatory: boolean
  addedAtCompact: number
  lastReminded: number
}

export type SessionDecisionRating = 'helpful' | 'correction'

export interface SessionDecisionFeedback {
  decisionId: string
  rating: SessionDecisionRating
  note?: string
  at: number
}

export interface SessionDecisions {
  sessionId: string
  entries: SessionDecisionEntry[]
  memories: SessionDecisionMemory[]
  selectedSkills: string[]
  selectedTools: string[]
  route?: { provider: string; model: string; pinned: boolean }
  compactCount: number
  feedback: SessionDecisionFeedback[]
}

const path = (sessionId: string) => `/api/sessions/${encodeURIComponent(sessionId)}/decisions`

export const sessionDecisionsApi = {
  read: (sessionId: string, signal?: AbortSignal): Promise<SessionDecisions> =>
    req<SessionDecisions>(path(sessionId), { signal }),
  pin: (sessionId: string, key: string, pinned: boolean): Promise<SessionDecisions> =>
    req<SessionDecisions>(`${path(sessionId)}/pin`, {
      method: 'PUT',
      body: JSON.stringify({ key, pinned }),
    }),
  feedback: (
    sessionId: string,
    decisionId: string,
    rating: SessionDecisionRating,
    note?: string,
  ): Promise<SessionDecisions> =>
    req<SessionDecisions>(`${path(sessionId)}/feedback`, {
      method: 'POST',
      body: JSON.stringify({ decisionId, rating, ...(note ? { note } : {}) }),
    }),
}
