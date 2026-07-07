// Artifacts — self-contained agent-produced content (workspace-scoped).
import type { Artifact, ArtifactKind } from '@/types'
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
    sourcePath?: string
    origin?: 'chat' | 'manual' | 'agent' | 'tool'
  }) =>
    req<Artifact>('/api/artifacts', { method: 'POST', body: JSON.stringify(data) }),
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
  // Locate the artifact on disk: its file path + containing folder.
  artifactPath: (id: string) =>
    req<{ path: string; dir: string }>(`/api/artifacts/${id}/path`),
  // Open the artifact's folder in the OS file manager (local desktop app).
  revealArtifact: (id: string) =>
    req<{ path: string; dir: string }>(`/api/artifacts/${id}/reveal`, { method: 'POST' }),
}
