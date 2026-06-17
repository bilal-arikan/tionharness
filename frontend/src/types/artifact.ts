// Artifacts — self-contained agent-produced content (documents, code, HTML,
// diagrams) viewed in a dedicated screen. Not versioned: updates overwrite.

export type ArtifactKind =
  | 'markdown'
  | 'code'
  | 'html'
  | 'text'
  | 'svg'
  | 'mermaid'
  // Media/file kinds: content lives on disk (a workspace-relative upload),
  // referenced by `sourcePath`, and is rendered inline (image/video/audio) or
  // shown as a stored-file card (file).
  | 'image'
  | 'video'
  | 'audio'
  | 'file'

export interface Artifact {
  id: string
  sessionId: string
  agentId: string
  title: string
  kind: ArtifactKind
  language: string
  content: string
  // Set when the artifact was auto-captured from a file the agent wrote.
  sourcePath?: string
  createdAt: number
  updatedAt: number
}

// The JSON result string returned by the create_artifact / update_artifact tools
// (parsed from a tool step's output to render an artifact card in chat).
export interface ArtifactRefResult {
  id: string
  title: string
  kind: ArtifactKind
  action: 'create' | 'update'
}
