import type { CatalogAgent } from '@/api/agentCatalog'

export type AgentKind = 'all' | 'custom' | 'services' | 'workers'
export function agentKind(row: CatalogAgent): AgentKind {
  if (!row.agent.system) return 'custom'
  return row.agent.systemKey?.startsWith('subagent-') ? 'workers' : 'services'
}
