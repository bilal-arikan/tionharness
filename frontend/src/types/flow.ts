// Orchestration flows: graph definition, persisted run state and run history (Phase 7).

export type FlowNodeType = 'agent' | 'branch' | 'parallel'

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
  branches?: FlowBranch[]
  parallel?: string[]
  joinNext?: string
}

export interface FlowGraph {
  start: string
  nodes: FlowNode[]
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
