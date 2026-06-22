// Per-agent memory: documents, journal, reflections and recall previews.
import type { Memory, MemoryKind, RecallHit } from '../types'
import { req } from './client'

export const memoryApi = {
  listMemories: (agentId: string, kind?: MemoryKind) =>
    req<Memory[]>(
      `/api/agents/${agentId}/memories${kind ? `?kind=${kind}` : ''}`,
    ),
  createMemory: (agentId: string, content: string, kind: MemoryKind = 'document') =>
    req<Memory>(`/api/agents/${agentId}/memories`, {
      method: 'POST',
      body: JSON.stringify({ content, kind }),
    }),
  deleteMemory: (id: string) =>
    req<{ result: string }>(`/api/memories/${id}`, { method: 'DELETE' }),
  // Core memory (MemGPT): the agent's editable working memory, split into a
  // persona section (about itself) and a human section (about the user).
  getCore: (agentId: string) =>
    req<{ persona: string; human: string }>(`/api/agents/${agentId}/core`),
  // Writes only the sections provided (each optional); a non-undefined empty
  // string clears that section. Returns the resulting persona/human.
  writeCore: (agentId: string, sections: { persona?: string; human?: string }) =>
    req<{ persona: string; human: string }>(`/api/agents/${agentId}/core`, {
      method: 'PUT',
      body: JSON.stringify(sections),
    }),
  reflect: (agentId: string) =>
    req<Memory>(`/api/agents/${agentId}/reflect`, { method: 'POST' }),
  recall: (agentId: string, query: string, limit = 5) =>
    req<RecallHit[]>(`/api/agents/${agentId}/recall`, {
      method: 'POST',
      body: JSON.stringify({ query, limit }),
    }),
}
