// Pure grouping/filtering helpers for the composer's tool inspector. No React,
// no state — kept out of ToolAccessPanel so that file stays layout-only.
import type { ToolAccessEntry } from '@/types'
import { compareText } from '@/shared/lib/intl'

// A tool's context state: whether its schema rides in the prompt right now
// ('in-context', always true for the eager tab) or only sits in the
// load-on-demand catalog until activated ('optional', the lazy tab's
// catalogued entries — 'hidden' tier tools also land here since they remain
// callable via tool_search/activate_tools, just uncatalogued).
export type ToolContextState = 'in-context' | 'optional'

export interface ToolContextGroup {
  key: ToolContextState
  label: string
  tools: ToolAccessEntry[]
}

const CONTEXT_GROUP_LABELS: Record<ToolContextState, string> = {
  'in-context': 'Bağlamda',
  optional: 'İsteğe bağlı',
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

// groupToolsByContext buckets tools by whether they occupy prompt context right
// now, NOT by where they come from — a built-in and an MCP tool with the same
// state land in the same group. 'in-context' always sorts first; empty buckets
// are dropped so an all-eager or all-optional list renders a single group.
export function groupToolsByContext(tools: ToolAccessEntry[]): ToolContextGroup[] {
  const buckets: Record<ToolContextState, ToolAccessEntry[]> = {
    'in-context': [],
    optional: [],
  }
  for (const t of tools) {
    buckets[t.inContext ? 'in-context' : 'optional'].push(t)
  }
  const order: ToolContextState[] = ['in-context', 'optional']
  return order
    .filter((key) => buckets[key].length > 0)
    .map((key) => ({
      key,
      label: CONTEXT_GROUP_LABELS[key],
      tools: [...buckets[key]].sort((a, b) => compareText(a.label, b.label)),
    }))
}
