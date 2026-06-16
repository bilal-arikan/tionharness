// Artifacts — versioned, self-contained agent-produced content (documents,
// code, HTML, diagrams) viewed in a dedicated screen.

export type ArtifactKind = 'markdown' | 'code' | 'html' | 'text' | 'svg' | 'mermaid'

export interface ArtifactRevision {
  version: number
  content: string
  note: string
  createdAt: number
}

export interface Artifact {
  id: string
  sessionId: string
  agentId: string
  title: string
  kind: ArtifactKind
  language: string
  content: string
  version: number
  revisions: ArtifactRevision[]
  createdAt: number
  updatedAt: number
}

// The JSON result string returned by the create_artifact / update_artifact tools
// (parsed from a tool step's output to render an artifact card in chat).
export interface ArtifactRefResult {
  id: string
  title: string
  kind: ArtifactKind
  version: number
  action: 'create' | 'update'
}
