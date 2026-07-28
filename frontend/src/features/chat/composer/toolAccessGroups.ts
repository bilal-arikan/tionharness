// Pure grouping/filtering helpers for the composer's tool inspector. No React,
// no state — kept out of ToolAccessPanel so that file stays layout-only.
import type { ToolAccessEntry } from '@/types'
import { CATEGORY_LABELS, CATEGORY_ORDER } from '@/features/tools/toolMeta'

export interface ToolGroup {
  // Stable key: "cat:<category>" for built-ins, "mcp:<server>" for MCP tools.
  key: string
  label: string
  source: 'builtin' | 'mcp'
  tools: ToolAccessEntry[]
}

// filterTools narrows a list by a free-text query matched against the tool name,
// its un-namespaced label, its server and its description. An empty query keeps
// everything.
export function filterTools(tools: ToolAccessEntry[], query: string): ToolAccessEntry[] {
  const q = query.trim().toLowerCase()
  if (!q) return tools
  return tools.filter((t) =>
    `${t.name} ${t.label} ${t.server} ${t.description}`.toLowerCase().includes(q),
  )
}

// groupTools buckets tools by functional category (built-ins) or by MCP server,
// mirroring how the full Tools screen organises them. Built-in groups come first
// in CATEGORY_ORDER, then MCP servers alphabetically.
export function groupTools(tools: ToolAccessEntry[]): ToolGroup[] {
  const groups = new Map<string, ToolGroup>()
  for (const t of tools) {
    const isMCP = t.source === 'mcp'
    const key = isMCP ? `mcp:${t.server}` : `cat:${t.category || 'other'}`
    let g = groups.get(key)
    if (!g) {
      g = {
        key,
        label: isMCP
          ? t.server || 'MCP'
          : CATEGORY_LABELS[t.category || 'other'] || t.category || 'other',
        source: isMCP ? 'mcp' : 'builtin',
        tools: [],
      }
      groups.set(key, g)
    }
    g.tools.push(t)
  }
  const rank = (g: ToolGroup) => {
    if (g.source === 'mcp') return CATEGORY_ORDER.length + 1
    const i = CATEGORY_ORDER.indexOf(g.key.slice('cat:'.length))
    return i === -1 ? CATEGORY_ORDER.length : i
  }
  return [...groups.values()].sort((a, b) => rank(a) - rank(b) || a.label.localeCompare(b.label))
}
