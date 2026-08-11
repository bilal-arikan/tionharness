import type { InstallResult, Pack, PackKind, Registry } from '@/types'
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

// WorkspaceExportInclude selects what a workspace-template export captures. The
// id/slug lists are tri-state: `null` means "every item", a present array
// (including `[]`) restricts to exactly its members (`[]` = none). The boolean
// flags gate the file categories. Omitting the whole object (publishPack without
// `include`) keeps the legacy "export everything" behaviour.
export interface WorkspaceExportInclude {
  agentIds: string[] | null
  flowIds: string[] | null
  skillSlugs: string[] | null
  scheduleIds: string[] | null
  /** Starter automation rules. Built-in seeded board defaults are excluded by the
   *  server regardless — every workspace provisions its own copy at open time. */
  automationIds: string[] | null
  instructions: boolean
  prompts: boolean // non-default runtime prompts + README
  boardColumns: boolean
}

// WorkspaceExportMeta carries optional pack-level metadata for a workspace
// export. Every field is optional; a blank value falls back to a server-derived
// default (name → workspace name, description → generated line, version → 1.0.0).
export interface WorkspaceExportMeta {
  name?: string
  description?: string
  version?: string
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
  publishPack: (
    kind: PackKind,
    sourceId: string,
    include?: WorkspaceExportInclude,
    meta?: WorkspaceExportMeta,
  ) =>
    req<Pack>('/api/market/publish', {
      method: 'POST',
      // meta (name/description/version) lives at the request top level next to
      // include; blank fields are dropped so the server applies its defaults.
      body: JSON.stringify({ kind, sourceId, include, ...meta }),
    }),
  importPack: (raw: string) => req<Pack>('/api/market/import', { method: 'POST', body: raw }),
}
