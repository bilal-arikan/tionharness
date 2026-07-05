// Pure helpers for the tools screen, split out of ToolsPanel to keep that file
// focused on state/behaviour. No React, no state — name parsing + schema flatten.
import type { ToolVisibility, WorkspaceTool } from '../../types'

// The four context-visibility tiers, in order of decreasing per-turn cost. Each
// entry drives the tier selector: short label, one-line hint, badge accent color.
export const VISIBILITY_TIERS: {
  value: ToolVisibility
  label: string
  hint: string
  color: string
}[] = [
  {
    value: 'full',
    label: 'Tam',
    hint: 'Tam şema her tur ajana gönderilir (en yüksek token maliyeti).',
    color: 'var(--color-success)',
  },
  {
    value: 'summary',
    label: 'Özet',
    hint: 'Katalogda isim + kısa özet görünür; tam şema gerektiğinde on-demand yüklenir.',
    color: 'var(--color-accent)',
  },
  {
    value: 'name-only',
    label: 'İsim',
    hint: 'Katalogda yalnız isim görünür (özet bastırılır); şema tool_search/activate_tools ile yüklenir.',
    color: 'var(--color-warning,#d97706)',
  },
  {
    value: 'hidden',
    label: 'Gizli',
    hint: 'Katalogda hiç listelenmez; yalnızca tool_search ile keşfedilir (yine de aktif edilince çağrılabilir).',
    color: 'var(--color-text-dim)',
  },
]

export function visibilityMeta(v: ToolVisibility | undefined) {
  return VISIBILITY_TIERS.find((t) => t.value === v) ?? VISIBILITY_TIERS[1]
}

// MCP tools are namespaced "<server>__<tool>". These helpers recover a tool's
// origin from its name when the backend doesn't supply source/server/label
// (e.g. an older server build), so built-ins always group correctly.
const NS_SEP = '__'

export function toolSource(t: WorkspaceTool): 'builtin' | 'mcp' {
  if (t.source) return t.source
  return t.name.includes(NS_SEP) ? 'mcp' : 'builtin'
}

export function toolServer(t: WorkspaceTool): string {
  if (t.server) return t.server
  const i = t.name.indexOf(NS_SEP)
  return i >= 0 ? t.name.slice(0, i) : ''
}

export function toolLabel(t: WorkspaceTool): string {
  if (t.label) return t.label
  const i = t.name.indexOf(NS_SEP)
  return i >= 0 ? t.name.slice(i + NS_SEP.length) : t.name
}

// Functional category for a built-in tool. Mirrors the backend keys in
// internal/tools/categories.go; falls back to "other" when the backend doesn't
// supply one (older build) so the tool still groups somewhere visible.
export function toolCategory(t: WorkspaceTool): string {
  return t.category && t.category.trim() ? t.category : 'other'
}

// CATEGORY_LABELS maps a backend category key to its Turkish display label. Keys
// not present here render under their raw key, so a new backend category degrades
// gracefully instead of disappearing.
export const CATEGORY_LABELS: Record<string, string> = {
  files: 'Dosya & Kabuk',
  search: 'Arama & Web',
  agents: 'Ajanlar & Oturumlar',
  automation: 'Otomasyon (Akış / Zamanlama / Görev)',
  interaction: 'Etkileşim',
  artifacts: 'Çıktılar (Artifacts)',
  'skills-mcp': 'Skill & MCP Yönetimi',
  config: 'Yapılandırma & Gizli Anahtarlar',
  diagnostics: 'Tanılama & Araç Yükleme',
  other: 'Diğer',
}

// CATEGORY_ORDER fixes the display order of built-in category groups (most-used
// first). Categories outside this list sort to the end, alphabetically by label.
export const CATEGORY_ORDER: string[] = [
  'files',
  'search',
  'agents',
  'automation',
  'interaction',
  'artifacts',
  'skills-mcp',
  'config',
  'diagnostics',
  'other',
]

// A JSON-Schema-ish parameter row extracted from a tool's inputSchema.
export interface ParamRow {
  name: string
  type: string
  required: boolean
  description: string
}

// extractParams flattens a tool's JSON Schema into a simple list for display.
export function extractParams(schema: unknown): ParamRow[] {
  if (!schema || typeof schema !== 'object') return []
  const s = schema as { properties?: Record<string, unknown>; required?: string[] }
  if (!s.properties) return []
  const required = new Set(s.required ?? [])
  return Object.entries(s.properties).map(([name, raw]) => {
    const p = (raw ?? {}) as { type?: string; description?: string }
    return {
      name,
      type: p.type ?? 'any',
      required: required.has(name),
      description: p.description ?? '',
    }
  })
}
