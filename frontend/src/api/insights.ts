// Retrospective session scanning (Insight, _Docs/60) — workspace-scoped.
import type {
  InsightLens,
  InsightFinding,
  InsightSettings,
  InsightRun,
  FleetFinding,
} from '@/types'
import { req } from './client'

export interface InsightScanRequest {
  lensIds?: string[]
  sessionAgentId?: string
  analysisAgentId?: string
  includeArchived?: boolean
  maxSessions?: number
}

export const insightApi = {
  listInsightLenses: () => req<InsightLens[]>('/api/insight/lenses'),

  // Starts a scan in the background (202). The result is not returned here — it
  // lands in the findings store + the "🔍 İçgörü Taraması" session. Poll
  // getInsightScanStatus to know when it finishes.
  runInsightScan: (body: InsightScanRequest = {}) =>
    req<{ started: boolean }>('/api/insight/scan', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  getInsightScanStatus: () => req<{ scanning: boolean }>('/api/insight/status'),

  listInsightFindings: (params?: { lens?: string; channel?: string }) => {
    const qs = new URLSearchParams()
    if (params?.lens) qs.set('lens', params.lens)
    if (params?.channel) qs.set('channel', params.channel)
    const q = qs.toString()
    return req<InsightFinding[]>(`/api/insight/findings${q ? `?${q}` : ''}`)
  },

  setInsightFindingStatus: (id: string, status: string) =>
    req<{ result: string; status: string }>(`/api/insight/findings/${id}/status`, {
      method: 'POST',
      body: JSON.stringify({ status }),
    }),

  deleteInsightFinding: (id: string) =>
    req<{ deleted: string }>(`/api/insight/findings/${id}`, { method: 'DELETE' }),

  // Clear accumulated insight data. deep=false keeps the ledger (board clears,
  // won't re-fill from old sessions); deep=true also clears it (re-scan from zero).
  resetInsight: (deep: boolean) =>
    req<{ reset: boolean; deep: boolean }>('/api/insight/reset', {
      method: 'POST',
      body: JSON.stringify({ deep }),
    }),

  getInsightSettings: () => req<InsightSettings>('/api/insight/settings'),

  updateInsightSettings: (s: InsightSettings) =>
    req<InsightSettings>('/api/insight/settings', {
      method: 'PUT',
      body: JSON.stringify(s),
    }),

  // Observability: recent scan-run log records (not sessions).
  getInsightRuns: () => req<InsightRun[]>('/api/insight/runs'),

  // Fleet-wide app-fix backlog, deduped across all workspaces.
  getFleetFindings: () => req<FleetFinding[]>('/api/insight/fleet-findings'),

  // Lens file editing.
  getLensRaw: (id: string) => req<{ id: string; raw: string }>(`/api/insight/lenses/${id}/raw`),
  updateLens: (id: string, raw: string) =>
    req<InsightLens>(`/api/insight/lenses/${id}`, { method: 'PUT', body: JSON.stringify({ raw }) }),
  toggleLens: (id: string, enabled: boolean) =>
    req<InsightLens>(`/api/insight/lenses/${id}/toggle`, {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
}
