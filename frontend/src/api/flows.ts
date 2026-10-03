// Evolving per-agent flows (_Docs/93).
import type {
  AgentPromptVersion,
  Flow,
  FlowGraph,
  FlowOptimizeResult,
  FlowPolicy,
  FlowProposal,
  FlowRun,
  FlowValidation,
  FlowVersion,
} from '@/types'
import { req } from './client'

export const flowApi = {
  listFlows: () => req<Flow[]>('/api/flows'),
  getFlow: (id: string) => req<Flow>(`/api/flows/${encodeURIComponent(id)}`),
  agentFlow: (agentId: string) => req<Flow>(`/api/agents/${encodeURIComponent(agentId)}/flow`),
  // Commit the canvas graph as a new version (author: user).
  saveFlow: (id: string, graph: FlowGraph, reason: string) =>
    req<{ flow: Flow; version?: FlowVersion; changed: boolean }>(
      `/api/flows/${encodeURIComponent(id)}`,
      { method: 'PUT', body: JSON.stringify({ graph, reason }) },
    ),
  updateFlowMeta: (id: string, patch: { name?: string; note?: string; policy?: FlowPolicy }) =>
    req<Flow>(`/api/flows/${encodeURIComponent(id)}/meta`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  validateFlow: (id: string, graph: FlowGraph) =>
    req<FlowValidation>(`/api/flows/${encodeURIComponent(id)}/validate`, {
      method: 'POST',
      body: JSON.stringify({ graph }),
    }),
  listFlowVersions: (id: string) =>
    req<FlowVersion[]>(`/api/flows/${encodeURIComponent(id)}/versions`),
  getFlowVersion: (id: string, n: number) =>
    req<FlowVersion>(`/api/flows/${encodeURIComponent(id)}/versions/${n}`),
  revertFlow: (id: string, version: number, reason = '') =>
    req<{ flow: Flow; version: FlowVersion }>(`/api/flows/${encodeURIComponent(id)}/revert`, {
      method: 'POST',
      body: JSON.stringify({ version, reason }),
    }),
  listFlowRuns: (id: string, limit = 50) =>
    req<FlowRun[]>(`/api/flows/${encodeURIComponent(id)}/runs?limit=${limit}`),
  getFlowRun: (runId: string) => req<FlowRun>(`/api/flow-runs/${encodeURIComponent(runId)}`),
  deleteFlowRun: (runId: string) =>
    req<{ result: string }>(`/api/flow-runs/${encodeURIComponent(runId)}`, { method: 'DELETE' }),
  // One observer pass now (manual trigger); may take a model call.
  optimizeFlow: (id: string) =>
    req<FlowOptimizeResult>(`/api/flows/${encodeURIComponent(id)}/optimize`, {
      method: 'POST',
      timeoutMs: 0,
    }),
  listFlowProposals: (id: string) =>
    req<FlowProposal[]>(`/api/flows/${encodeURIComponent(id)}/proposals`),
  applyFlowProposal: (proposalId: string) =>
    req<FlowProposal>(`/api/flow-proposals/${encodeURIComponent(proposalId)}/apply`, {
      method: 'POST',
    }),
  rejectFlowProposal: (proposalId: string) =>
    req<FlowProposal>(`/api/flow-proposals/${encodeURIComponent(proposalId)}/reject`, {
      method: 'POST',
    }),
  // Run the flow once on a test input in a fresh, tagged chat session.
  testFlow: (id: string, input: string) =>
    req<{ sessionId: string; flowId: string; agentId: string }>(
      `/api/flows/${encodeURIComponent(id)}/test`,
      { method: 'POST', body: JSON.stringify({ input }) },
    ),
  listPromptVersions: (agentId: string) =>
    req<AgentPromptVersion[]>(`/api/agents/${encodeURIComponent(agentId)}/prompt-versions`),
  restorePromptVersion: (agentId: string, n: number) =>
    req<{ restored: number }>(
      `/api/agents/${encodeURIComponent(agentId)}/prompt-versions/${n}/restore`,
      { method: 'POST' },
    ),
}
