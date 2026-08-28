// Pure grouping/filtering helpers for the composer's tool inspector. No React,
// no state — kept out of ToolAccessPanel so that file stays layout-only.
import type { ToolAccessEntry } from '@/types'
import { compareText } from '@/shared/lib/intl'

// sortTools orders a tool list the way the inspector shows it: alphabetically by
// un-namespaced label, regardless of source. Built-in and MCP tools mix freely —
// the tab already answers "is it in context", so source is a per-row badge, not a
// grouping key. Returns a new array; the input is left untouched.
export function sortTools(tools: ToolAccessEntry[]): ToolAccessEntry[] {
  return [...tools].sort((a, b) => compareText(a.label, b.label))
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

// toolsForServer picks the tools a single MCP server contributes, in the order
// the inspector shows them: eager tools first (their schema is in every turn),
// then the load-on-demand ones, alphabetical by un-namespaced label inside each
// bucket so the list is stable regardless of API ordering.
export function toolsForServer(
  tools: { eager: ToolAccessEntry[]; lazy: ToolAccessEntry[] },
  serverName: string,
): ToolAccessEntry[] {
  const pick = (list: ToolAccessEntry[]) =>
    list
      .filter((t) => t.source === 'mcp' && t.server === serverName)
      .sort((a, b) => compareText(a.label, b.label))
  return [...pick(tools.eager), ...pick(tools.lazy)]
}
