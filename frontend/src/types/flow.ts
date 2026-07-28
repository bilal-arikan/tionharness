// Orchestration flows: graph definition, persisted run state and run history (Phase 7).

export type FlowNodeType =
  | 'agent'
  | 'branch'
  | 'parallel'
  | 'delay'
  | 'transform'
  | 'loop'
  | 'await-input'
  | 'subflow'
  | 'start'
  | 'end'
  | 'spawn'
  | 'join'
  | 'coordinator'

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
  jsonField?: string // branch: match on this top-level JSON field of the value (structured routing)
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
  // await-input node: 0/absent = wait forever; else fail the run after N seconds.
  // coordinator node: how long to wait for the coordinator to settle (0 = default).
  timeoutSec?: number
  // coordinator node — cap on the coordinator's auto-turn (notify-loop) budget
  // for this node only (0/absent = the recipe's own cap, else the workspace default).
  maxTurns?: number
  // coordinator node — saved coordination recipe slug (a skill with kind
  // 'coordinator-workflow'; the same list the session info panel offers).
  // ''/absent = free coordination.
  workflow?: string
  // subflow node
  flowRef?: string // id of the child flow to run
  // spawn node — launch these flows as async child runs (non-blocking)
  spawnFlows?: string[]
  // join node — which spawn node's children to await ("" = all outstanding)
  spawnRef?: string
  joinTimeoutSec?: number // 0/absent = wait forever
  joinPartial?: boolean // drop failed/suspended/timed-out children instead of failing the join
  // end node — optional output contract
  outputSchema?: string // JSON Schema the final output must satisfy (else the run fails)
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

// One accumulated-thread message (mirrors orchestration.Msg). In accumulate mode
// each agent node's prior context is FlowState.thread sliced to the node's
// threadLen.
export interface FlowMsg {
  role: string // "user" | "assistant"
  text: string
}

export interface FlowTraceEntry {
  nodeId: string
  type: string
  title: string
  output: string
  // Rendered prompt actually sent to an agent node (after {{...}} substitution),
  // or the evaluated value for a branch node. Absent for other non-agent nodes.
  input?: string
  // Accumulate mode: how many thread messages this agent node saw as prior
  // context before its own turn (FlowState.thread[:threadLen]). Absent/0 otherwise.
  threadLen?: number
  // Unix-ms execution bounds, populated for parallel children so the run
  // inspector can draw a concurrency timeline. Absent for sequential nodes.
  startMs?: number
  endMs?: number
  at: number
}

export interface FlowState {
  current: string
  last: string
  outputs: Record<string, string>
  steps: number
  trace: FlowTraceEntry[]
  // Accumulated conversation for accumulate-mode runs (grows per agent node). A
  // node's prior context is thread[:entry.threadLen]. Empty on stateless runs.
  thread?: FlowMsg[]
  // Set to the await-input node id while the run is durably suspended for input.
  waitingAt?: string
}

// FlowNodeEvent is one node's live lifecycle frame, broadcast over the SSE
// `flownode` channel (backend orchestration.NodeEvent) while a run executes so
// the run viewer shows per-node start/done/error + output the moment it happens,
// ahead of the periodic run-state poll.
export interface FlowNodeEvent {
  phase: 'start' | 'done' | 'error' | 'waiting' | 'progress'
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
  // Per-run transcript session this run produced (Session.kind "flow"). Lets the
  // chat "Akış olarak gör" resolve a flow session back to its exact run + real
  // graph. Empty on runs recorded before the link existed.
  sessionId?: string
  status: 'running' | 'success' | 'failure' | 'waiting'
  input: string
  state: string // JSON FlowState
  output: string
  error: string
  createdAt: number
  updatedAt: number
}
