// Orchestration flows: graph definition, persisted run state and run history (Phase 7).

export type FlowNodeType = 'agent' | 'branch' | 'parallel' | 'delay' | 'transform' | 'loop'

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
  fresh?: boolean // agent node: opt out of the accumulated thread (accumulate mode)
  // loop node
  body?: string // entry node id of the loop body
  loopNext?: string // node after the loop exits
  maxIters?: number // hard iteration cap (0 = rely on until)
  until?: string // exit when {{last}} matches
  untilMode?: BranchMatchMode // how until matches (default: contains)
  // Cosmetic canvas layout (persisted; ignored by the engine).
  x?: number
  y?: number
}

export interface FlowGraph {
  start: string
  nodes: FlowNode[]
  // Accumulate mode: sequential agent nodes share a growing conversation thread
  // so the provider's prompt cache reuses the stable prefix across nodes.
  accumulate?: boolean
  // Cosmetic canvas presentation (persisted; ignored by the engine).
  edgeStyle?: string // default | smoothstep | step | straight
  animated?: boolean
}

export interface Flow {
  id: string
  name: string
  emoji?: string // optional cosmetic glyph shown wherever the flow is listed/picked
  graph: string // JSON FlowGraph
  tags?: string[] // free-form organizational labels (editable by user + agents)
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

// FlowNodeEvent is one node's live lifecycle frame, broadcast over the SSE
// `flownode` channel (backend orchestration.NodeEvent) while a run executes so
// the run viewer shows per-node start/done/error + output the moment it happens,
// ahead of the periodic run-state poll.
export interface FlowNodeEvent {
  phase: 'start' | 'done' | 'error'
  nodeId: string
  type: string
  title: string
  index: number // 1-based execution order
  output?: string // on "done"
  error?: string // on "error"
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
