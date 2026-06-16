// Artifacts — versioned agent-produced content (workspace-scoped).
import type { Artifact, ArtifactKind } from '../types'
import { req } from './client'

export const artifactApi = {
  listArtifacts: (sessionId?: string) =>
    req<Artifact[]>(
      sessionId ? `/api/artifacts?sessionId=${encodeURIComponent(sessionId)}` : '/api/artifacts',
    ),
  getArtifact: (id: string) => req<Artifact>(`/api/artifacts/${id}`),
  createArtifact: (data: {
    title: string
    kind?: ArtifactKind
    language?: string
    content?: string
    sessionId?: string
    agentId?: string
  }) =>
    req<Artifact>('/api/artifacts', { method: 'POST', body: JSON.stringify(data) }),
  updateArtifact: (
    id: string,
    patch: { content?: string; note?: string; title?: string; kind?: ArtifactKind; language?: string },
  ) => req<Artifact>(`/api/artifacts/${id}`, { method: 'PUT', body: JSON.stringify(patch) }),
  deleteArtifact: (id: string) =>
    req<{ ok: boolean }>(`/api/artifacts/${id}`, { method: 'DELETE' }),
}
