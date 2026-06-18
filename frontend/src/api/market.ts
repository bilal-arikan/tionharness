import type { InstallResult, Pack, PackKind } from '../types'
import { req } from './client'

export const marketApi = {
  listMarket: (kind?: PackKind) =>
    req<Pack[]>(`/api/market${kind ? `?kind=${encodeURIComponent(kind)}` : ''}`),
  getPack: (id: string) => req<Pack>(`/api/market/${encodeURIComponent(id)}`),
  reloadMarket: () => req<void>('/api/market/reload', { method: 'POST' }),
  installPack: (id: string, body?: { overwrite?: boolean; apiKey?: string }) =>
    req<InstallResult>(`/api/market/${encodeURIComponent(id)}/install`, {
      method: 'POST',
      body: JSON.stringify(body ?? {}),
    }),
  publishPack: (kind: PackKind, sourceId: string) =>
    req<Pack>('/api/market/publish', {
      method: 'POST',
      body: JSON.stringify({ kind, sourceId }),
    }),
  importPack: (raw: string) =>
    req<Pack>('/api/market/import', { method: 'POST', body: raw }),
}
