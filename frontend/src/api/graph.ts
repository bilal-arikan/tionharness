// Relationship-graph endpoints: the workspace collaboration network and the
// per-agent memory knowledge graph.
import type { WorkspaceGraph, MemoryGraph } from '../types'
import { req } from './client'

export const graphApi = {
  // Workspace collaboration network: agents/tasks/flows + their relationships.
  workspaceGraph: () => req<WorkspaceGraph>('/api/graph'),
  // Per-agent memory knowledge graph. threshold is the lexical-cosine floor for
  // an edge; max caps the number of (strongest) edges returned.
  memoryGraph: (agentId: string, threshold = 0.18, max = 400) =>
    req<MemoryGraph>(
      `/api/agents/${agentId}/memory-graph?threshold=${threshold}&max=${max}`,
    ),
}
