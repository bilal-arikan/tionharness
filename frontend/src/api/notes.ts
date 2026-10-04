// Workspace memory (notes) + awareness layer — workspace-scoped (_Docs/94).
import type {
  AwarenessSeen,
  AwarenessSettings,
  Digest,
  DigestIndexEntry,
  Note,
  NoteExpansion,
  NoteSearchHit,
  NoteStats,
  NoteWrite,
} from '@/types'
import { req } from './client'

export interface ListNotesParams {
  // Comma-joined server-side; an empty list means every kind.
  kinds?: string[]
  scope?: string
  source?: string
  tag?: string
  session?: string
  agent?: string
  // Include archived / superseded (retired) notes. The default list excludes
  // both; private notes are always included for the human.
  archived?: boolean
  retired?: boolean
  since?: number
  limit?: number
}

function noteQuery(params: ListNotesParams): URLSearchParams {
  const p = new URLSearchParams()
  if (params.kinds?.length) p.set('kind', params.kinds.join(','))
  if (params.scope) p.set('scope', params.scope)
  if (params.source) p.set('source', params.source)
  if (params.tag) p.set('tag', params.tag)
  if (params.session) p.set('session', params.session)
  if (params.agent) p.set('agent', params.agent)
  if (params.archived) p.set('archived', 'true')
  if (params.retired) p.set('retired', 'true')
  if (params.since) p.set('since', String(params.since))
  if (params.limit !== undefined) p.set('limit', String(params.limit))
  return p
}

export const noteApi = {
  // Newest first. Archived + superseded notes are excluded unless asked for.
  listNotes: (params: ListNotesParams = {}, signal?: AbortSignal): Promise<Note[]> => {
    const qs = noteQuery(params).toString()
    return req<Note[]>(qs ? `/api/notes?${qs}` : '/api/notes', { signal })
  },
  noteStats: () => req<NoteStats>('/api/notes/stats'),
  // Lexical search: every query token must appear in title, body or tags.
  searchNotes: (
    q: string,
    params: ListNotesParams = {},
    signal?: AbortSignal,
  ): Promise<NoteSearchHit[]> => {
    const p = noteQuery(params)
    p.set('q', q)
    return req<NoteSearchHit[]>(`/api/notes/search?${p.toString()}`, { signal })
  },
  getNote: (id: string) => req<Note>(`/api/notes/${encodeURIComponent(id)}`),
  // Links, backlinks and the correction chain around one note.
  expandNote: (id: string, signal?: AbortSignal) =>
    req<NoteExpansion>(`/api/notes/${encodeURIComponent(id)}/expand`, { signal }),
  createNote: (data: NoteWrite) =>
    req<Note>('/api/notes', { method: 'POST', body: JSON.stringify(data) }),
  // Partial: an empty string keeps the stored value.
  updateNote: (id: string, data: Partial<NoteWrite>) =>
    req<Note>(`/api/notes/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  // 409 when the note is linked or sits in a correction chain ("archive it instead").
  deleteNote: (id: string) =>
    req<{ result: string }>(`/api/notes/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  setNoteArchived: (id: string, archived: boolean) =>
    req<Note>(`/api/notes/${encodeURIComponent(id)}/archive`, {
      method: 'POST',
      body: JSON.stringify({ archived }),
    }),
  // Writes the replacement note; the old one becomes supersededBy the new id.
  correctNote: (id: string, data: NoteWrite) =>
    req<Note>(`/api/notes/${encodeURIComponent(id)}/correct`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Awareness: session digests + what a session's agent was told.
  listDigests: (limit?: number, signal?: AbortSignal) =>
    req<DigestIndexEntry[]>(
      limit ? `/api/awareness/digests?limit=${limit}` : '/api/awareness/digests',
      { signal },
    ),
  awarenessSettings: () => req<AwarenessSettings>('/api/awareness/settings'),
  // 404 when the session has no digest yet.
  sessionDigest: (sessionId: string, signal?: AbortSignal) =>
    req<Digest>(`/api/sessions/${encodeURIComponent(sessionId)}/digest`, { signal }),
  sessionAwareness: (sessionId: string, signal?: AbortSignal) =>
    req<AwarenessSeen>(`/api/sessions/${encodeURIComponent(sessionId)}/awareness`, { signal }),
}
