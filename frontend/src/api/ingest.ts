import { req } from './client'

// Generic import (ingest) pipeline: scan a foreign source (GitHub repo/plugin or a
// local folder tree), then bulk-install the selected artifacts (skills/agents/
// commands/MCP/hooks) through the same install authority the market uses. (SK-IMP3)

export type IngestKind = 'skill' | 'agent' | 'flow' | 'provider' | 'workspace' | 'mcp' | 'hook'

// Discovered is one artifact found in a source, with preview metadata + a unique
// selection key.
export interface Discovered {
  key: string
  kind: IngestKind
  slug: string
  name: string
  description: string
  relPath: string
  files: string[]
  warnings?: string[]
  exists: boolean
}

export interface IngestScanResult {
  source: string
  location: string
  items: Discovered[]
  warnings: string[]
}

export interface IngestSource {
  source: 'github' | 'local'
  path?: string // local folder (source=local)
  url?: string // github repo/tree URL or owner/repo (source=github)
}

export interface IngestInstallInput extends IngestSource {
  keys?: string[] // selected Discovered.key values (empty = all)
  slugPrefix?: string
  shared?: boolean
  group?: string // Skills-UI group for imported skills (keeps them from mixing with existing ones)
}

export interface InstallResult {
  kind: string
  ref: string
  message: string
}

export interface IngestSkipNote {
  key: string
  slug: string
  reason: string
}

export interface IngestInstallResult {
  message?: string
  installed: InstallResult[]
  skipped: IngestSkipNote[]
  warnings: string[]
}

// PreviewItem is a discovered artifact with its rendered body, for the detail view
// of a source-ref (directory-site) catalog entry before install.
export interface PreviewItem {
  kind: IngestKind
  slug: string
  name: string
  description: string
  body: string
  files: string[]
  warnings?: string[]
}

export const ingestApi = {
  ingestScan: (input: IngestSource) =>
    req<IngestScanResult>('/api/ingest/scan', { method: 'POST', body: JSON.stringify(input) }),
  ingestPreview: (input: IngestSource) =>
    req<{ items: PreviewItem[] }>('/api/ingest/preview', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  ingestInstall: (input: IngestInstallInput) =>
    req<IngestInstallResult>('/api/ingest/install', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
}
