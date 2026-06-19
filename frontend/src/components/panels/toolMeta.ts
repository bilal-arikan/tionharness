// Pure helpers for the tools screen, split out of ToolsPanel to keep that file
// focused on state/behaviour. No React, no state — name parsing + schema flatten.
import type { WorkspaceTool } from '../../types'

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
