// Unified executions feed: every run path (chat/task/flow/schedule) funnels its
// output into a Session, so this single list surfaces them all with kind + live
// status. Backed by GET /api/executions.
import type { Execution, ExecutionRuntimeRow } from '@/types'
import { req } from './client'

export const executionApi = {
  // List executions across all agents, newest-updated first. Optional kind
  // filters to a single category (chat | task | flow | schedule).
  listExecutions: (kind?: string) =>
    req<Execution[]>(kind ? `/api/executions?kind=${encodeURIComponent(kind)}` : '/api/executions'),
  // Compact live facts for the sidebar poll: only sessions that are running,
  // carry a last-run status or sit under a coordinator. Absent = idle.
  listExecutionRuntime: () => req<ExecutionRuntimeRow[]>('/api/executions/runtime'),
}
