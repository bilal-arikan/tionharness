// User-added custom OpenAI/Anthropic-compatible providers (app-global). The
// list is managed independently of the main settings save flow.
import { req } from './client'

export interface CustomProvider {
  id: string
  label: string
  kind: 'openai' | 'anthropic'
  baseUrl: string
  defaultModel: string
  models: string
  keySet: boolean
  // Capability metadata: reasoning-effort support + prompt-cache behaviour
  // ("native" | "auto" | "none" | undefined = unknown).
  reasoning?: boolean
  promptCache?: string
}

// UpsertProviderInput mirrors the backend payload. key is write-only: omit to
// keep the stored key, '' to clear it, a value to replace it.
export interface UpsertProviderInput {
  id: string
  label: string
  kind: string
  baseUrl: string
  defaultModel: string
  models: string
  key?: string
  reasoning?: boolean
  promptCache?: string
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
  listCustomProviders: () => req<CustomProvider[]>('/api/providers'),
  upsertCustomProvider: (p: UpsertProviderInput) =>
    req<CustomProvider[]>('/api/providers', { method: 'PUT', body: JSON.stringify(p) }),
  deleteCustomProvider: (id: string) =>
    req<CustomProvider[]>(`/api/providers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  prices: () => req<PriceTable>('/api/prices'),
}
