// Budget screen data: today's workspace-wide spend with a per-origin breakdown,
// a per-agent table, and a daily trend. Mirrors GET /api/usage.

export interface KindStat {
  calls: number
  inputTokens: number
  outputTokens: number
}

export interface BudgetAgentRow {
  agentId: string
  name: string
  avatar?: string
  color?: string
  provider?: string
  calls: number
  inputTokens: number
  outputTokens: number
  byKind?: Record<string, KindStat>
  costUSD: number
  priced: boolean
  dailyCallLimit: number
  dailyTokenLimit: number
}

export interface ProviderStat {
  provider: string
  calls: number
  inputTokens: number
  outputTokens: number
  costUSD: number
  priced: boolean
}

export interface BudgetTrendPoint {
  day: string
  calls: number
  inputTokens: number
  outputTokens: number
}

export interface WorkspaceUsage {
  day: string
  totals: {
    calls: number
    inputTokens: number
    outputTokens: number
    byKind: Record<string, KindStat>
    costUSD: number
    priced: boolean
  }
  byProvider: ProviderStat[]
  agents: BudgetAgentRow[]
  trend: BudgetTrendPoint[]
}
