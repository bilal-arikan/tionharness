// Shared types mirroring the Go backend JSON models.

export interface Workspace {
  id: string
  name: string
  createdAt: number
}

export interface Agent {
  id: string
  name: string
  soul: string
  identity: string
  provider: string
  model: string
  planningMode: string
  createdAt: number
  updatedAt: number
}

export interface Session {
  id: string
  agentId: string
  kind: string
  title: string
  messageCount: number
  state: string
  createdAt: number
  updatedAt: number
}

export interface Message {
  id: string
  sessionId: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  text: string
  createdAt: number
}

export interface Usage {
  inputTokens: number
  outputTokens: number
}

export interface ChatResponse {
  reply: string
  usage: Usage
  model: string
  userMessage: Message
  replyMessage: Message
}

// Kanban board column states (mirror db.Board* constants).
export type BoardState = 'todo' | 'in_progress' | 'review' | 'done' | 'failed'

export interface Task {
  id: string
  title: string
  description: string
  prompt: string
  ownerAgentId: string
  boardState: BoardState
  dependencies: string
  lastRunId: string
  lastRunStatus: string
  lastRunAt: number
  createdAt: number
  updatedAt: number
}

export interface Run {
  id: string
  taskId: string
  agentId: string
  status: 'pending' | 'running' | 'success' | 'failure'
  trigger: string
  output: string
  error: string
  createdAt: number
  updatedAt: number
}

export interface Schedule {
  id: string
  agentId: string
  taskId: string
  cronExpr: string
  prompt: string
  nextRunAt: number
  lastRunAt: number
  lastDeliveryStatus: string
  lastDeliveryError: string
  enabled: boolean
  createdAt: number
}

export type MemoryKind = 'document' | 'journal' | 'reflection'

export interface Memory {
  id: string
  agentId: string
  kind: MemoryKind
  content: string
  createdAt: number
}

export interface RecallHit {
  id: string
  kind: MemoryKind
  content: string
  score: number
}

export interface SessionContext {
  contextTokens: number
  hasSummary: boolean
  summaryMsgCount: number
  messageCount: number
}

export interface AgentUsage {
  day: string
  calls: number
  inputTokens: number
  outputTokens: number
  dailyCallLimit: number
  dailyTokenLimit: number
}
