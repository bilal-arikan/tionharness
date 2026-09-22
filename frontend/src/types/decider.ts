// Decision-model layer (backend: internal/decider). Mirrors the JSON of
// GET/PUT /api/decider, the /api/decider/models routes, the test calls and
// GET /api/decider/stats.

export type DeciderMode = 'off' | 'shadow' | 'on'
export type DeciderPattern = 'gate' | 'pick' | 'rate' | 'select' | 'triage'
export type DeciderCredentials = 'own' | 'provider'

export interface DeciderAuthorityConfig {
  mode: DeciderMode
  threshold: number
  model?: string
  fallback?: string
  challenger?: string
}

export interface DeciderConfig {
  enabled: boolean
  defaultModel: string
  authorities: Record<string, DeciderAuthorityConfig>
}

export interface DeciderModelSuggestion {
  id: string
  label: string
  description?: string
  inputPerMTok: number
  outputPerMTok: number
}

export interface DeciderField {
  key: string
  label: string
  type: 'text' | 'number' | 'textarea' | 'select'
  options?: string[]
  default?: string
  placeholder?: string
  help?: string
}

export interface DeciderPreset {
  id: string
  label: string
  description?: string
  credentials: DeciderCredentials
  baseUrl?: string
  model: string
  contextTokens?: number
  timeoutMs?: number
  config?: Record<string, string>
}

export interface DeciderBackend {
  id: string
  label: string
  description?: string
  providerKinds: string[]
  defaultBaseUrl: string
  keyRequired: boolean
  fields?: DeciderField[]
  models: DeciderModelSuggestion[]
  defaultModel: string
  contextTokens: number
  defaultTimeoutMs: number
  limits: { maxOptions?: number; maxLevels?: number }
  modelPrefixes?: string[]
  decisionOnly: boolean
  calibrated: boolean
  presets?: DeciderPreset[]
}

export interface DeciderAuthority {
  id: string
  group: string
  pattern: DeciderPattern
  label: string
  description: string
  modes: DeciderMode[]
  defaultMode: DeciderMode
  defaultThreshold: number
  thresholdHint: string
  explicit: boolean
  failClosed?: boolean
  order?: number
}

export interface DeciderProviderInstance {
  id: string
  kind: string
  label: string
  baseUrl?: string
  enabled: boolean
  available: boolean
}

export interface DeciderModelStatus {
  ready: boolean
  problem?: string
  problemAt?: number
  backoffUntil?: number
  provider?: string
  endpoint?: string
}

export interface DeciderModelStats {
  instance: string
  calls: number
  errors: number
  p50Ms: number
  p95Ms: number
  costUsd: number
  lastAt?: number
}

export interface DeciderModelInstance {
  id: string
  label: string
  backend: string
  enabled: boolean
  model: string
  credentials: DeciderCredentials
  providerInstanceId: string
  baseUrl: string
  timeoutMs: number
  contextTokens: number
  config: Record<string, string>
  secretsSet: Record<string, boolean>
  createdAt: string
  updatedAt: string
  status: DeciderModelStatus
  stats?: DeciderModelStats
  usedBy: string[]
}

// DeciderModelInput is the create/update body. secrets is write-only: a key left
// out keeps the stored value, an empty string clears it.
export interface DeciderModelInput {
  id?: string
  label: string
  backend: string
  enabled: boolean
  model: string
  credentials: DeciderCredentials
  providerInstanceId: string
  baseUrl: string
  timeoutMs: number
  contextTokens: number
  config: Record<string, string>
  secrets?: Record<string, string>
}

export interface DeciderStatus {
  enabled: boolean
  model?: string
  ready: boolean
  problem?: string
  problemAt?: number
  backoffUntil?: number
}

export interface DeciderAuthorityStats {
  authority: string
  calls: number
  errors: number
  shadow: number
  compared: number
  agreed: number
  applied: number
  fallbacks: number
  p50Ms: number
  p95Ms: number
  costUsd: number
  lastAt?: number
  challengerCalls: number
  challengerErrors: number
  challengerCompared: number
  challengerAgreed: number
  challengerP50Ms: number
  challengerCostUsd: number
  challenger?: string
}

export interface DeciderRecord {
  at: number
  authority: string
  mode: DeciderMode
  role?: 'challenger'
  instance?: string
  model?: string
  fallback?: boolean
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
  groups: string[]
  authorities: DeciderAuthority[]
  models: DeciderModelInstance[]
  providerCandidates: Record<string, DeciderProviderInstance[]>
  stats: DeciderAuthorityStats[]
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
  instance?: string
  model: string
  servedModel?: string
  billingProvider?: string
  billingModel?: string
  fallback?: boolean
  warnings?: string[]
  answers: Record<string, DeciderAnswer>
  usage: { inputTokens: number; outputTokens: number; costUsd: number }
  latencyMs: number
}

export interface DeciderTestResult {
  ok: boolean
  error?: string
  response?: DeciderResponse
}

export interface DeciderModelSaved {
  model: DeciderModelInstance
  view: DeciderView
}

export interface DeciderModelDeleted {
  deleted: boolean
  usedBy: string[]
  view: DeciderView
}
