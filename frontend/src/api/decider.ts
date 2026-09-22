// Decision-model layer settings (backend: internal/api/decider.go). App-global:
// the decider is one service per process, not per workspace.
import { req } from './client'
import type {
  DeciderConfig,
  DeciderRecord,
  DeciderSiteStats,
  DeciderTestResult,
  DeciderView,
} from '@/types/decider'

export const deciderApi = {
  getDecider: () => req<DeciderView>('/api/decider'),
  saveDecider: (config: DeciderConfig) =>
    req<DeciderView>('/api/decider', { method: 'PUT', body: JSON.stringify(config) }),
  testDecider: () => req<DeciderTestResult>('/api/decider/test', { method: 'POST' }),
  getDeciderStats: (days: number) =>
    req<{ stats: DeciderSiteStats[]; recent: DeciderRecord[]; statsDays: number }>(
      `/api/decider/stats?days=${encodeURIComponent(String(days))}`,
    ),
}
