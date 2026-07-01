import type { InstallResult, Pack, PackKind, Registry } from '../types'
import { req } from './client'

// ConnectorInfo describes a built-in directory-site connector (skillsmp …) for the
// quick-add list. Adding one enables it as a registry that ingests on install.
export interface ConnectorInfo {
  id: string
  name: string
  url: string
  detail: string
}

// ConnectorSearchResult is the live directory-site search response: matching
// source-ref packs + per-connector warnings.
export interface ConnectorSearchResult {
  results: Pack[]
  warnings: string[]
}

// WorkspaceExportInclude selects what a workspace-template export captures.
// `agentIds: null` means every agent; the boolean flags gate each optional
// category. Omitting the whole object (publishPack without `include`) keeps the
// legacy "export everything" behaviour.
export interface WorkspaceExportInclude {
  agentIds: string[] | null
  flows: boolean
  schedules: boolean
  skills: boolean
  instructions: boolean
  boardColumns: boolean
}

export const marketApi = {
  listRegistries: () => req<Registry[]>('/api/market/registries'),
  addRegistry: (name: string, url: string) =>
    req<Registry[]>('/api/market/registries', {
      method: 'POST',
      body: JSON.stringify({ name, url }),
    }),
  removeRegistry: (url: string) =>
    req<Registry[]>('/api/market/registries/delete', {
      method: 'POST',
      body: JSON.stringify({ url }),
    }),
  refreshRegistries: () => req<void>('/api/market/registries/refresh', { method: 'POST' }),
  listConnectors: () => req<ConnectorInfo[]>('/api/market/connectors'),
  searchConnectors: (q: string, limit = 40) =>
    req<ConnectorSearchResult>(
      `/api/market/connectors/search?q=${encodeURIComponent(q)}&limit=${limit}`,
    ),
  listMarket: (kind?: PackKind) =>
    req<Pack[]>(`/api/market${kind ? `?kind=${encodeURIComponent(kind)}` : ''}`),
  getPack: (id: string) => req<Pack>(`/api/market/${encodeURIComponent(id)}`),
  reloadMarket: () => req<void>('/api/market/reload', { method: 'POST' }),
  installPack: (id: string, body?: { overwrite?: boolean; apiKey?: string; agentId?: string }) =>
    req<InstallResult>(`/api/market/${encodeURIComponent(id)}/install`, {
      method: 'POST',
      body: JSON.stringify(body ?? {}),
    }),
  publishPack: (kind: PackKind, sourceId: string, include?: WorkspaceExportInclude) =>
    req<Pack>('/api/market/publish', {
      method: 'POST',
      body: JSON.stringify({ kind, sourceId, include }),
    }),
  importPack: (raw: string) =>
    req<Pack>('/api/market/import', { method: 'POST', body: raw }),
}
