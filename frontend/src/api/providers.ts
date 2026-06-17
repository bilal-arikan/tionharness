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
}

export const providerApi = {
  listCustomProviders: () => req<CustomProvider[]>('/api/providers'),
  upsertCustomProvider: (p: UpsertProviderInput) =>
    req<CustomProvider[]>('/api/providers', { method: 'PUT', body: JSON.stringify(p) }),
  deleteCustomProvider: (id: string) =>
    req<CustomProvider[]>(`/api/providers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
}
