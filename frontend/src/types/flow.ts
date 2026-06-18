// Orchestration flows: graph definition, persisted run state and run history (Phase 7).

export type FlowNodeType = 'agent' | 'branch' | 'parallel' | 'delay' | 'transform'

export type BranchMatchMode = 'contains' | 'equals' | 'regex'

export interface FlowBranch {
  contains: string // case-insensitive substring; "" = default
  next: string
}

export interface FlowNode {
  id: string
  type: FlowNodeType
  title?: string
  agentId?: string
  prompt?: string
  next?: string
  branches?: FlowBranch[] // branch routing arms ("" contains = default)
  matchMode?: BranchMatchMode // branch: how arm values match (default: contains)
  parallel?: string[]
  joinNext?: string
  delayMs?: number // delay node: ms to wait
  template?: string // transform node: rendered output template
  // Cosmetic canvas layout (persisted; ignored by the engine).
  x?: number
  y?: number
}

export interface FlowGraph {
  start: string
  nodes: FlowNode[]
  // Cosmetic canvas presentation (persisted; ignored by the engine).
  edgeStyle?: string // default | smoothstep | step | straight
  animated?: boolean
}

export interface Flow {
  id: string
  name: string
  description: string
  graph: string // JSON FlowGraph
  createdAt: number
  updatedAt: number
}

export interface FlowTraceEntry {
  nodeId: string
  type: string
  title: string
  output: string
  at: number
}

export interface FlowState {
  current: string
  last: string
  outputs: Record<string, string>
  steps: number
  trace: FlowTraceEntry[]
}

export interface FlowRun {
  id: string
  flowId: string
  status: 'running' | 'success' | 'failure'
  input: string
  state: string // JSON FlowState
  output: string
  error: string
  createdAt: number
  updatedAt: number
}
