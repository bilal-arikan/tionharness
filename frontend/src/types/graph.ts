// Relationship-graph payloads: the workspace collaboration network and the
// per-agent memory knowledge graph. Both feed the vis-network canvas.

// A node in the workspace collaboration network. `id` is type-prefixed by the
// backend ("agent:…" / "task:…" / "flow:…").
export type WorkspaceNodeType = 'agent' | 'task' | 'flow' | 'skill' | 'mcp'

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

// A node in an agent's memory knowledge graph (one per memory).
export interface MemoryGraphNode {
  id: string
  kind: 'document' | 'journal' | 'reflection'
  content: string
  createdAt: number
  degree: number
}

export interface MemoryGraphEdge {
  source: string
  target: string
  score: number // lexical-cosine similarity in [0,1]
}

export interface MemoryGraph {
  nodes: MemoryGraphNode[]
  edges: MemoryGraphEdge[]
}
