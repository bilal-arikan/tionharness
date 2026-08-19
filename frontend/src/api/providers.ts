// Provider kinds (taslak) + provider instances (örnek), app-global. See
// _Docs/71-SAGLAYICI-ORNEKLERI-PLANI.md for the backend contract this mirrors.
import { req } from './client'

// FieldSpec is one field of a provider kind's instance form, mirroring the
// backend's providerFieldSpecDTO. No kind or field is hard-coded on the
// frontend — every form is rendered from this.
export interface ProviderFieldSpec {
  key: string
  label: string
  type: 'text' | 'password' | 'path' | 'dir' | 'select'
  options?: string[]
  required: boolean
  default?: string
  placeholder?: string
  help?: string
  secret: boolean
}

export interface CatalogModelInfo {
  id: string
  label?: string
  description?: string
  contextWindow?: number
}

// ProviderKind is one registered provider kind (taslak) with its full instance
// form, from GET /api/provider-kinds.
export interface ProviderKind {
  id: string
  label: string
  transport: 'api' | 'cli'
  multi: boolean
  fields: ProviderFieldSpec[]
  models?: CatalogModelInfo[]
}

// ProviderInstance is a user-configured provider account (örnek), from
// GET /api/providers. Secrets are never present — only secretsSet.
export interface ProviderInstance {
  id: string
  kindId: string
  label: string
  icon: string
  enabled: boolean
  defaultModel: string
  models: string
  config: Record<string, string>
  secretsSet: Record<string, boolean>
  createdAt: string
}

// UpsertProviderInput mirrors settings.ProviderInstanceInput. Secrets follow
// the write-only convention: a key absent from `secrets` keeps the stored
// value, an empty value clears it, a non-empty value replaces it.
export interface UpsertProviderInput {
  id?: string
  kindId: string
  label: string
  icon?: string
  enabled: boolean
  defaultModel?: string
  models?: string
  config: Record<string, string>
  secrets: Record<string, string>
}

export interface DeleteProviderResult {
  deleted: boolean
  affectedAgents: string[]
}

export interface ProviderAuthStatus {
  kind: 'claude-cli' | 'codex-cli'
  loggedIn: boolean
  installed: boolean
  homeDir: string
  tier?: string
  detail?: string
}

export interface ClaudeOAuthStartResult {
  flowId: string
  authUrl: string
}

export interface ClaudeOAuthCompleteResult {
  ok: boolean
  claudeHomeDir: string
  expiresAt: number
  subscription?: string
}

export interface ClaudeOAuthLoopbackStartResult extends ClaudeOAuthStartResult {
  port: number
}

export interface ClaudeOAuthLoopbackStatus {
  status: 'pending' | 'ok' | 'error' | 'unknown'
  detail?: string
  claudeHomeDir?: string
}

export interface CodexDeviceStartResult {
  verifyUrl: string
  code: string
  expiresInSec: number
}

export interface CodexDeviceStatus {
  state: 'pending' | 'success' | 'failed' | 'expired' | 'cancelled'
  error?: string
}

// ModelPrice is one model's approximate list price (USD per 1M tokens).
export interface ModelPrice {
  inputPerMTok: number
  outputPerMTok: number
  cacheReadMult?: number
  cacheWriteMult?: number
}

// PriceTable maps provider id → model id → price. Ballpark figures from the
// backend (providers.priceTable), surfaced so the UI can show $/1M-token costs.
export type PriceTable = Record<string, Record<string, ModelPrice>>

export const providerApi = {
  listProviderKinds: () => req<ProviderKind[]>('/api/provider-kinds'),
  listProviders: () => req<ProviderInstance[]>('/api/providers'),
  getProvider: (id: string) => req<ProviderInstance>(`/api/providers/${encodeURIComponent(id)}`),
  upsertProvider: (input: UpsertProviderInput) =>
    req<ProviderInstance>('/api/providers', { method: 'PUT', body: JSON.stringify(input) }),
  deleteProvider: (id: string) =>
    req<DeleteProviderResult>(`/api/providers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  getAuth: (id: string) => req<ProviderAuthStatus>(`/api/providers/${encodeURIComponent(id)}/auth`),
  startClaudeOAuth: (id: string) =>
    req<ClaudeOAuthStartResult>(`/api/providers/${encodeURIComponent(id)}/auth/oauth/start`, {
      method: 'POST',
    }),
  completeClaudeOAuth: (id: string, flowId: string, code: string) =>
    req<ClaudeOAuthCompleteResult>(`/api/providers/${encodeURIComponent(id)}/auth/oauth/complete`, {
      method: 'POST',
      body: JSON.stringify({ flowId, code }),
    }),
  startClaudeOAuthLoopback: (id: string) =>
    req<ClaudeOAuthLoopbackStartResult>(
      `/api/providers/${encodeURIComponent(id)}/auth/oauth/loopback/start`,
      { method: 'POST' },
    ),
  claudeOAuthLoopbackStatus: (id: string, flowId: string) =>
    req<ClaudeOAuthLoopbackStatus>(
      `/api/providers/${encodeURIComponent(id)}/auth/oauth/loopback/status?flowId=${encodeURIComponent(flowId)}`,
    ),
  startCodexDeviceAuth: (id: string) =>
    req<CodexDeviceStartResult>(`/api/providers/${encodeURIComponent(id)}/auth/device/start`, {
      method: 'POST',
    }),
  codexDeviceAuthStatus: (id: string) =>
    req<CodexDeviceStatus>(`/api/providers/${encodeURIComponent(id)}/auth/device/status`),
  cancelCodexDeviceAuth: (id: string) =>
    req<{ ok: boolean }>(`/api/providers/${encodeURIComponent(id)}/auth/device/cancel`, {
      method: 'POST',
    }),
  codexAPIKeyLogin: (id: string, apiKey: string) =>
    req<{ ok: boolean }>(`/api/providers/${encodeURIComponent(id)}/auth/api-key`, {
      method: 'POST',
      body: JSON.stringify({ apiKey }),
    }),
  prices: () => req<PriceTable>('/api/prices'),
}
