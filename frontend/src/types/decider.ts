// Decision-model layer (backend: internal/decider). Mirrors the JSON of
// GET/PUT /api/decider, POST /api/decider/test and GET /api/decider/stats.

export type DeciderMode = 'off' | 'shadow' | 'on'

export interface DeciderSiteConfig {
  mode: DeciderMode
  threshold: number
}

export interface DeciderConfig {
  enabled: boolean
  backend: string
  providerInstanceId: string
  model: string
  timeoutMs: number
  sites: Record<string, DeciderSiteConfig>
}

export interface DeciderModel {
  id: string
  label: string
  description?: string
  inputPerMTok: number
  outputPerMTok: number
}

export interface DeciderBackend {
  id: string
  label: string
  description?: string
  providerKinds: string[]
  billingProvider: string
  models: DeciderModel[]
  defaultModel: string
  contextTokens: number
  modelPrefixes?: string[]
}

export interface DeciderSite {
  id: string
  label: string
  description: string
  modes: DeciderMode[]
  defaultMode: DeciderMode
  defaultThreshold: number
  thresholdHint: string
  explicit: boolean
}

export interface DeciderInstance {
  id: string
  kind: string
  label: string
  baseUrl?: string
  enabled: boolean
  available: boolean
}

export interface DeciderStatus {
  enabled: boolean
  backend: string
  model: string
  instance?: string
  ready: boolean
  problem?: string
  problemAt?: number
  backoffUntil?: number
}

export interface DeciderSiteStats {
  site: string
  calls: number
  errors: number
  shadow: number
  compared: number
  agreed: number
  applied: number
  p50Ms: number
  p95Ms: number
  costUsd: number
  lastAt?: number
}

export interface DeciderRecord {
  at: number
  site: string
  mode: DeciderMode
  model?: string
  latencyMs?: number
  inputTokens?: number
  costUsd?: number
  outcome?: string
  strength?: number
  baseline?: string
  applied?: boolean
  error?: string
  ref?: string
}

export interface DeciderView {
  config: DeciderConfig
  status: DeciderStatus
  backends: DeciderBackend[]
  sites: DeciderSite[]
  candidates: DeciderInstance[]
  stats: DeciderSiteStats[]
  recent: DeciderRecord[]
  statsDays: number
}

export interface DeciderAnswer {
  type: 'noul' | 'choice' | 'score'
  probability?: number
  choice?: string
  score?: number
  probabilities?: Record<string, number>
  confidence?: number
}

export interface DeciderResponse {
  id?: string
  backend: string
  model: string
  servedModel?: string
  billingProvider?: string
  answers: Record<string, DeciderAnswer>
  usage: { inputTokens: number; outputTokens: number; costUsd: number }
  latencyMs: number
}

export interface DeciderTestResult {
  ok: boolean
  error?: string
  response?: DeciderResponse
}
