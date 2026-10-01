// Decision-model layer settings (backend: internal/api/decider.go and
// decider_models.go). App-global: the decider is one service per process, not
// per workspace.
import { req } from './client'
import type {
  DeciderConfig,
  DeciderModelDeleted,
  DeciderModelInput,
  DeciderModelSaved,
  DeciderRecord,
  DeciderAuthorityStats,
  DeciderTestResult,
  DeciderView,
  DeciderDebugFilter,
  DeciderDebugReport,
} from '@/types/decider'

const modelPath = (id: string) => `/api/decider/models/${encodeURIComponent(id)}`

export const deciderApi = {
  getDeciderDebug: (filter: DeciderDebugFilter = {}) => {
    const query = new URLSearchParams()
    for (const [key, value] of Object.entries(filter)) {
      if (value !== undefined && value !== '') query.set(key, String(value))
    }
    return req<DeciderDebugReport>(`/api/decider/debug?${query}`)
  },
  getDecider: () => req<DeciderView>('/api/decider'),
  saveDecider: (config: DeciderConfig) =>
    req<DeciderView>('/api/decider', { method: 'PUT', body: JSON.stringify(config) }),
  testDecider: () => req<DeciderTestResult>('/api/decider/test', { method: 'POST' }),
  getDeciderStats: (days: number) =>
    req<{ stats: DeciderAuthorityStats[]; recent: DeciderRecord[]; statsDays: number }>(
      `/api/decider/stats?days=${encodeURIComponent(String(days))}`,
    ),
  createDeciderModel: (input: DeciderModelInput) =>
    req<DeciderModelSaved>('/api/decider/models', { method: 'POST', body: JSON.stringify(input) }),
  updateDeciderModel: (id: string, input: DeciderModelInput) =>
    req<DeciderModelSaved>(modelPath(id), { method: 'PUT', body: JSON.stringify(input) }),
  deleteDeciderModel: (id: string) => req<DeciderModelDeleted>(modelPath(id), { method: 'DELETE' }),
  testDeciderModel: (id: string) =>
    req<DeciderTestResult>(`${modelPath(id)}/test`, { method: 'POST' }),
}
