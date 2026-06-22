// Per-agent memory: documents, journal, reflections and recall previews.
import type { CoreBlock, Memory, MemoryKind, RecallHit } from '../types'
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
  // Core memory (MemGPT): the agent's editable working memory as named blocks
  // (persona + human by default, plus any custom blocks).
  getCore: (agentId: string) =>
    req<{ blocks: CoreBlock[] }>(`/api/agents/${agentId}/core`),
  // Writes content for the given block labels (keyed by label). Returns the
  // resulting blocks. Over-limit / unknown labels are rejected (400).
  writeCore: (agentId: string, blocks: Record<string, string>) =>
    req<{ blocks: CoreBlock[] }>(`/api/agents/${agentId}/core`, {
      method: 'PUT',
      body: JSON.stringify({ blocks }),
    }),
  // Defines or updates a named block (seeds persona/human first when none).
  defineCoreBlock: (
    agentId: string,
    def: { label: string; description?: string; charLimit?: number; readOnly?: boolean },
  ) =>
    req<{ blocks: CoreBlock[] }>(`/api/agents/${agentId}/core/blocks`, {
      method: 'POST',
      body: JSON.stringify(def),
    }),
  // Deletes a block definition and its content.
  deleteCoreBlock: (agentId: string, label: string) =>
    req<{ blocks: CoreBlock[] }>(`/api/agents/${agentId}/core/blocks/${encodeURIComponent(label)}`, {
      method: 'DELETE',
    }),
  reflect: (agentId: string) =>
    req<Memory>(`/api/agents/${agentId}/reflect`, { method: 'POST' }),
  recall: (agentId: string, query: string, limit = 5) =>
    req<RecallHit[]>(`/api/agents/${agentId}/recall`, {
      method: 'POST',
      body: JSON.stringify({ query, limit }),
    }),
}
