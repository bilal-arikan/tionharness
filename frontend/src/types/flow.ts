// Evolving per-agent flows (_Docs/93): one main flow per agent, versioned,
// with one run per turn, observer proposals and prompt versions. Mirrors
// internal/flow (graph) and internal/db/models_flow.go (store rows).

export type FlowNodeType = 'input' | 'llm' | 'route' | 'transform' | 'trigger' | 'output'
export type FlowContextMode = 'thread' | 'fresh'
export type FlowToolsMode = 'inherit' | 'none'
export type RouteMode = 'contains' | 'equals' | 'regex' | 'json' | 'judge' | 'criteria'
export type FlowPolicyMode = 'off' | 'propose' | 'auto'
export type FlowRunStatus = 'running' | 'success' | 'failure'
export type FlowProposalStatus = 'pending' | 'applied' | 'rejected' | 'invalid'
export type FlowAuthorKind = 'user' | 'agent' | 'observer' | 'system'

export interface FlowNode {
  id: string
  type: FlowNodeType
  title?: string
  note?: string
  x?: number
  y?: number
  // llm
  agentId?: string
  prompt?: string
  context?: FlowContextMode
  tools?: FlowToolsMode
  outputSchema?: string
  model?: string
  // route
  mode?: RouteMode
  jsonField?: string
  question?: string
  criteria?: string[] // criteria mode: yes/no statements the last output must satisfy
  maxVisits?: number
  // trigger
  automationId?: string
  // transform / output / trigger (payload)
  template?: string
}

export interface FlowEdge {
  id: string
  from: string
  to: string
  when?: string
}

export interface FlowGraph {
  version: number
  nodes: FlowNode[]
  edges: FlowEdge[]
  maxSteps?: number
}

export interface FlowAuthor {
  kind: FlowAuthorKind
  id?: string
}

export interface FlowPolicy {
  mode: FlowPolicyMode
  everyRuns: number
  minConfidence: number
  maxNodes: number
  allowPromptChanges: boolean
}

export interface FlowStats {
  runs: number
  success: number
  failure: number
  lastRunAt?: number
  lastRunId?: string
  totalMs: number
  totalTokens: number
  runsAtOptimize: number
  lastOptimizeAt?: number
  // Decision-model grades (1..5) folded in by the flow-grade authority.
  graded?: number
  gradeSum?: number
}

export interface Flow {
  id: string
  agentId: string
  name: string
  graph: string // JSON FlowGraph (head version)
  version: number
  policy: FlowPolicy
  stats: FlowStats
  note?: string
  createdAt: number
  updatedAt: number
  // List enrichment (GET /api/flows)
  agentName: string
  agentAvatar?: string
  agentColor?: string
  agentArchived?: boolean
  shape: string
  trivial: boolean
  nodeCount: number
  pendingProposals: number
}

export interface FlowVersion {
  flowId: string
  version: number
  parent?: number
  graph: string
  author: FlowAuthor
  reason?: string
  proposalId?: string
  diff?: string
  createdAt: number
}

export interface FlowRunStep {
  index: number
  nodeId: string
  type: FlowNodeType | string
  title?: string
  visit: number
  input?: string
  output?: string
  edge?: string
  detail?: string // criteria verdicts / what a trigger launched
  startedAt: number // unix ms
  endedAt: number
  durationMs: number
  error?: string
}

export interface FlowRunUsage {
  inputTokens: number
  outputTokens: number
  llmCalls: number
}

export interface FlowRun {
  id: string
  flowId: string
  agentId: string
  sessionId?: string
  messageId?: string
  version: number
  trigger?: string
  status: FlowRunStatus
  input: string
  output?: string
  error?: string
  steps?: FlowRunStep[] | null
  stepCount: number
  durationMs: number
  usage: FlowRunUsage
  feedback?: number
  // Decision-model grade of the reply (1..5) and the confidence behind it.
  grade?: number
  gradeConfidence?: number
  createdAt: number
  updatedAt: number
}

export interface FlowPromptChange {
  soul?: string | null
  identity?: string | null
}

export interface FlowProposal {
  id: string
  flowId: string
  agentId: string
  baseVersion: number
  author: FlowAuthor
  trigger?: string
  ops?: unknown[] | null
  prompt?: FlowPromptChange | null
  reason: string
  expected?: string
  confidence: number
  evidence?: string
  status: FlowProposalStatus
  appliedVersion?: number
  error?: string
  createdAt: number
  resolvedAt?: number
}

export interface AgentPromptVersion {
  agentId: string
  version: number
  soul: string
  identity: string
  author: FlowAuthor
  reason?: string
  proposalId?: string
  createdAt: number
}

// One live node frame (SSE `flownode`, backend flow.Event).
export interface FlowNodeEvent {
  phase: 'start' | 'done' | 'error'
  nodeId: string
  type: string
  title: string
  index: number
  visit: number
  output?: string
  edge?: string
  detail?: string
  error?: string
  durationMs?: number
}

export interface FlowNodeFrame {
  runId: string
  flowId: string
  agentId: string
  sessionId?: string
  event: FlowNodeEvent
}

export interface FlowOptimizeResult {
  flowId: string
  trigger: string
  ran: boolean
  skipped?: string
  proposal?: FlowProposal | null
  applied: boolean
  // Why the proposal gate kept a confident proposal pending.
  held?: string
  sessionId?: string
}

export interface FlowValidation {
  ok: boolean
  error?: string
  shape?: string
  trivial?: boolean
  nodeCount?: number
}
