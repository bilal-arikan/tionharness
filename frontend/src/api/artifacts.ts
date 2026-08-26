// Artifacts — self-contained agent-produced content (workspace-scoped).
import type { Artifact, ArtifactKind } from '@/types'
import { req } from './client'

// ArtifactPage is the paged listing envelope returned by listArtifacts — the
// API twin of the agent tool's pageResult (TSK68 scope extension). A
// parameter-less call still receives the legacy unwrapped array on the wire and
// is normalized to this shape client-side, so every caller can rely on
// items/total/hasMore.
export interface ArtifactPage {
  items: Artifact[]
  total: number
  offset: number
  limit: number
  hasMore: boolean
}

// asArtifactPage normalizes either wire shape (unwrapped legacy array or paged
// envelope) to the paged form.
export function asArtifactPage(r: Artifact[] | ArtifactPage): ArtifactPage {
  return Array.isArray(r)
    ? { items: r, total: r.length, offset: 0, limit: r.length, hasMore: false }
    : r
}

export interface ListArtifactsParams {
  sessionId?: string
  // q filters by title substring (case-insensitive, server-side).
  q?: string
  kind?: string
  origin?: string
  archived?: boolean
  limit?: number
  offset?: number
}

export const artifactApi = {
  // Always resolves to the paged envelope: when paging params are given the
  // server replies {items,total,offset,limit,hasMore}; when they are absent it
  // replies the legacy unwrapped array, normalized here.
  listArtifacts: (params: ListArtifactsParams = {}): Promise<ArtifactPage> => {
    const p = new URLSearchParams()
    if (params.sessionId) p.set('sessionId', params.sessionId)
    if (params.q) p.set('q', params.q)
    if (params.kind) p.set('kind', params.kind)
    if (params.origin) p.set('origin', params.origin)
    if (params.archived !== undefined) p.set('archived', params.archived ? 'true' : 'false')
    if (params.limit !== undefined) p.set('limit', String(params.limit))
    if (params.offset !== undefined) p.set('offset', String(params.offset))
    const qs = p.toString()
    return req<Artifact[] | ArtifactPage>(qs ? `/api/artifacts?${qs}` : '/api/artifacts').then(
      asArtifactPage,
    )
  },
  getArtifact: (id: string) => req<Artifact>(`/api/artifacts/${id}`),
  createArtifact: (data: {
    title: string
    kind?: ArtifactKind
    language?: string
    content?: string
    sessionId?: string
    agentId?: string
    sourcePath?: string
    origin?: 'chat' | 'manual' | 'agent' | 'tool'
  }) => req<Artifact>('/api/artifacts', { method: 'POST', body: JSON.stringify(data) }),
  updateArtifact: (
    id: string,
    patch: { content?: string; title?: string; kind?: ArtifactKind; language?: string },
  ) => req<Artifact>(`/api/artifacts/${id}`, { method: 'PUT', body: JSON.stringify(patch) }),
  deleteArtifact: (id: string) =>
    req<{ ok: boolean }>(`/api/artifacts/${id}`, { method: 'DELETE' }),
  // Assign an artifact's `group` (its Artifacts-UI organisation bucket) without
  // touching any other field. Empty string ungroups it. Drives the bulk "set
  // group" action, mirroring the Skills screen's grouping.
  setArtifactGroup: (id: string, group: string) =>
    req<Artifact>(`/api/artifacts/${id}/group`, {
      method: 'PUT',
      body: JSON.stringify({ group }),
    }),
  // Archive / un-archive an artifact — a soft, reversible hide (the artifact is
  // never deleted). Archived artifacts drop out of the default list and show
  // only behind the "archived" filter, from where they can be restored.
  setArtifactArchived: (id: string, archived: boolean) =>
    req<Artifact>(`/api/artifacts/${id}/archive`, {
      method: 'PUT',
      body: JSON.stringify({ archived }),
    }),
  // Locate the artifact on disk: its file path + containing folder.
  artifactPath: (id: string) => req<{ path: string; dir: string }>(`/api/artifacts/${id}/path`),
}
