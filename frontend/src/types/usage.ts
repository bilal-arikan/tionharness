// Budget screen data: today's workspace-wide spend with a per-origin breakdown,
// a per-agent table, and a daily trend. Mirrors GET /api/usage.

export interface KindStat {
  calls: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens?: number
  cacheWriteTokens?: number
}

export interface ModelStat {
  model: string
  calls: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  costUSD: number
  savingsUSD: number
  priced: boolean
  estimated?: boolean // equivalent-API estimate for subscription providers (e.g. claude-cli)
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
  estimated?: boolean // equivalent-API estimate for subscription providers (e.g. claude-cli)
  dailyCallLimit: number
  dailyTokenLimit: number
}

export interface ProviderStat {
  provider: string
  calls: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  costUSD: number
  savingsUSD: number
  priced: boolean
  estimated?: boolean // equivalent-API estimate for subscription providers (e.g. claude-cli)
  models: ModelStat[]
}

export interface BudgetTrendPoint {
  day: string
  calls: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  costUSD: number
  savingsUSD: number
}

// Window-cumulative totals across the selected trend window ("oturumlar arası
// toplam" / caching ROI). cacheHitRate is cacheRead / (cacheRead + input +
// cacheWrite) — the share of prompt tokens served from cache.
export interface BudgetCumulative {
  days: number
  calls: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  costUSD: number
  savingsUSD: number
  cacheHitRate: number
}

export interface WorkspaceUsage {
  day: string
  totals: {
    calls: number
    inputTokens: number
    outputTokens: number
    cacheReadTokens: number
    cacheWriteTokens: number
    byKind: Record<string, KindStat>
    costUSD: number
    savingsUSD: number
    priced: boolean
    estimated?: boolean // true when cost includes equivalent-API estimates (e.g. claude-cli)
  }
  byProvider: ProviderStat[]
  agents: BudgetAgentRow[]
  trend: BudgetTrendPoint[]
  cumulative: BudgetCumulative
}
