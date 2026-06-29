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
  // Tool-output compaction savings (bytes) for this agent today.
  compactSavedBytes?: number
  compactSavedBytesLLM?: number
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
  compactSavedBytes: number
  compactSavedBytesLLM: number
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
  compactSavedBytes: number
  compactSavedBytesLLM: number
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
    compactSavedBytes: number    // System A: deterministic tool-output trim
    compactSavedBytesLLM: number // System B: LLM summary trim
  }
  byProvider: ProviderStat[]
  agents: BudgetAgentRow[]
  trend: BudgetTrendPoint[]
  cumulative: BudgetCumulative
}

// Per-session lifetime spend + savings — GET /api/sessions/{id}/usage-detail.
// The session-scoped analog of AgentUsage.
export interface SessionUsageDetail {
  sessionId: string
  agentId: string
  calls: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  byKind?: Record<string, KindStat>
  byModel?: ModelStat[]
  costUSD: number
  savingsUSD: number
  priced: boolean
  estimated?: boolean
  compactSavedBytes: number
  compactSavedBytesLLM: number
}

// Per-session debug journal — GET /api/sessions/{id}/debug. The parallel
// observability stream (separate from the conversation): turn timings, per-call
// token spend, per-tool latency/size/errors, hook decisions, compaction/recovery.
export interface SessionDebugToolStat {
  calls: number
  errors: number
  durMs: number
  outBytes: number
}

export interface SessionDebugSummary {
  sessionId: string
  events: number
  turns: number
  llmCalls: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  toolCalls: number
  errors: number
  compactions: number
  recoveries: number
  savedBytes: number
  turnDurMs: number
  byTool?: Record<string, SessionDebugToolStat>
  byModel?: Record<string, number>
  topTools?: string[]
  lastError?: string
  firstTs?: number
  lastTs?: number
  turnDurSeries?: number[]
  tokenSeries?: number[]
  anomalies?: SessionDebugAnomaly[]
}

export interface SessionDebugAnomaly {
  severity: 'warn' | 'info'
  code: string
  message: string
}

export interface SessionDebugEvent {
  ts: number
  type: 'turn' | 'llm_call' | 'tool' | 'hook' | 'error' | 'compaction' | 'recovery'
  sessionId?: string
  turnId?: string
  agentId?: string
  kind?: string
  name?: string
  model?: string
  durMs?: number
  in?: number
  out?: number
  cacheRead?: number
  cacheWrite?: number
  outBytes?: number
  savedBytes?: number
  stop?: string
  err?: boolean
  detail?: string
}

// One tool execution within a single turn (per-message debug panel row).
export interface TurnToolCall {
  name: string
  durMs: number
  outBytes: number
  err?: boolean
}

// Per-MESSAGE debug rollup behind the chat message debug button: the token spend,
// latency, model, cost and the exact tool calls that produced one assistant reply.
// Backed by GET /api/sessions/{id}/turn-debug?turn={replyMessageId}.
export interface TurnDebug {
  sessionId: string
  turnId: string
  found: boolean
  model?: string
  durMs: number
  stop?: string
  llmCalls: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  toolCalls: number
  tools?: TurnToolCall[]
  errors: number
  recoveries: number
  compactions: number
  lastError?: string
  costUSD: number
  savingsUSD: number
  priced: boolean
  estimated: boolean
  firstTs?: number
  lastTs?: number
}
