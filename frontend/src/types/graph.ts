// Relationship-graph payloads: the workspace collaboration network.
// Feeds the vis-network canvas.

// A node in the workspace collaboration network. `id` is type-prefixed by the
// backend ("agent:<agentId>#<sessionId>" / "task:…" / "flow:…").
export type WorkspaceNodeType = 'agent' | 'task' | 'flow' | 'skill' | 'mcp' | 'run'

export interface WorkspaceGraphNode {
  id: string
  type: WorkspaceNodeType
  label: string
  sub?: string
  color?: string // agent accent — drives the cluster hue
  emoji?: string // agent avatar glyph, if any
  group?: string // owning hub id (cluster layout)
  status?: string // task board state
  desc?: string // longer description (task tooltip)
  // Live activity (agents): an agent node IS a running instance — one per
  // in-flight session — so `running` is always true and the same agent can
  // appear several times, once per session it is driving.
  running?: boolean
  runKind?: string // chat | task | flow | schedule | spawned | worker | inbox
  runTarget?: string // type-prefixed id of the running task/flow (or empty)
  sessionId?: string // the session behind this agent instance
  agentId?: string // agent definition id shared by all instances of an agent
}

export interface WorkspaceGraphEdge {
  source: string
  target: string
  kind: 'owns' | 'created' | 'runs' | 'uses' | 'skill' | 'mcp'
}

export interface WorkspaceGraph {
  nodes: WorkspaceGraphNode[]
  edges: WorkspaceGraphEdge[]
  stats: Record<string, number>
}
