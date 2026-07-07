// Relationship-graph endpoints: the workspace collaboration network.
import type { WorkspaceGraph } from '@/types'
import { req } from './client'

export const graphApi = {
  // Workspace collaboration network: agents/tasks/flows + their relationships.
  workspaceGraph: () => req<WorkspaceGraph>('/api/graph'),
}
