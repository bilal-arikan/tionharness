// Unified executions feed: every run path (chat/task/flow/schedule) funnels its
// output into a Session, so this single list surfaces them all with kind + live
// status. Backed by GET /api/executions.
import type { Execution } from '../types'
import { req } from './client'

export const executionApi = {
  // List executions across all agents, newest-updated first. Optional kind
  // filters to a single category (chat | task | flow | schedule | heartbeat).
  listExecutions: (kind?: string) =>
    req<Execution[]>(
      kind ? `/api/executions?kind=${encodeURIComponent(kind)}` : '/api/executions',
    ),
}
