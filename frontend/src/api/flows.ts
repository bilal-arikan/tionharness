// Orchestration flows and their run history (Phase 7).
import type { Flow, FlowGraph, FlowRun } from '../types'
import { req } from './client'

export const flowApi = {
  listFlows: () => req<Flow[]>('/api/flows'),
  createFlow: (name: string, description = '', graph?: FlowGraph) =>
    req<Flow>('/api/flows', {
      method: 'POST',
      body: JSON.stringify({ name, description, graph }),
    }),
  updateFlow: (id: string, name: string, description: string, graph: FlowGraph) =>
    req<Flow>(`/api/flows/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ name, description, graph }),
    }),
  deleteFlow: (id: string) =>
    req<{ result: string }>(`/api/flows/${id}`, { method: 'DELETE' }),
  runFlow: (id: string, input: string) =>
    req<FlowRun>(`/api/flows/${id}/run`, {
      method: 'POST',
      body: JSON.stringify({ input }),
    }),
  listFlowRuns: (flowId: string) =>
    req<FlowRun[]>(`/api/flow-runs?flowId=${encodeURIComponent(flowId)}`),
  getFlowRun: (id: string) => req<FlowRun>(`/api/flow-runs/${id}`),
}
