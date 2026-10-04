// Workspace memory (notes) + the awareness layer — mirrors internal/notes and
// internal/awareness (_Docs/94).
//
// A note is a Markdown memory with a frontmatter block. It declares its REACH
// when written (scope + agents/projects), carries a confidence tag and is never
// overwritten by a correction: the replacement supersedes it, and the retired
// note is served only next to its successor. Agents archive notes, never delete.

export type NoteKind =
  'lesson' | 'decision' | 'work' | 'gotcha' | 'pattern' | 'profile' | 'reference'

export type NoteScope = 'agent' | 'project' | 'workspace'

export type NoteConfidence = 'verified' | 'inferred' | 'unverified'

export interface Note {
  id: string
  kind: NoteKind
  title: string
  scope: NoteScope
  agents?: string[]
  projects?: string[]
  confidence: NoteConfidence
  // How a verified note was checked. Required for 'verified'.
  verification?: string
  // Correction chain: supersedes points back, supersededBy marks the note retired.
  supersedes?: string
  supersededBy?: string
  created: number // unix seconds
  updated: number // unix seconds
  // Provenance stamped by the runtime, never claimed by the writer.
  sourceSession?: string
  sourceAgent?: string
  source?: string
  // Dedupe key of machine-written notes (a lesson's failure shape); a repeat
  // bumps occurrences instead of adding a near-duplicate.
  signature?: string
  occurrences?: number
  tags?: string[]
  // Private notes are the user's own: never served to an agent.
  private?: boolean
  archived?: boolean
  body: string
  // [[wikilink]] targets found in body (raw, unresolved).
  links?: string[]
}

// The user-authored shape accepted by POST/PUT/correct. PUT is partial: an empty
// string keeps the stored value.
export interface NoteWrite {
  kind: NoteKind
  title: string
  body: string
  scope: NoteScope
  agents?: string[]
  projects?: string[]
  confidence: NoteConfidence
  verification?: string
  tags?: string[]
  private?: boolean
}

export interface NoteStats {
  total: number
  active: number
  retired: number
  archived: number
  private: number
  byKind: Record<string, number>
  byScope: Record<string, number>
  newest: number
  sources: Record<string, number>
}

export interface NoteSearchHit {
  note: Note
  score: number
  snippet: string
}

// GET /api/notes/{id}/expand — the note's neighbourhood.
export interface NoteExpansion {
  note: Note
  links: Note[]
  unresolved?: string[]
  backlinks: Note[]
  // Predecessors walk supersedes backwards (oldest last); successors walk
  // supersededBy forwards (newest last).
  predecessors?: Note[]
  successors?: Note[]
}

// ---- awareness --------------------------------------------------------------

// Per-workspace awareness layer settings (workspace settings `awareness`).
export interface AwarenessSettings {
  enabled: boolean
  briefBudgetBytes: number
  turnBudgetBytes: number
  pulseBudgetBytes: number
  digestBudgetBytes: number
  recentSessions: number
  recentDigests: number
  noteCount: number
  staleCardDays: number
}

export interface DigestRef {
  id: string
  title: string
}

export interface DigestToolCount {
  name: string
  calls: number
  errors?: number
}

export interface DigestTodo {
  total: number
  done: number
  inProgress: number
  open?: string[]
}

// A session's end-of-turn digest (GET /api/sessions/{id}/digest).
export interface Digest {
  sessionId: string
  agentId?: string
  agentName?: string
  title: string
  kind?: string
  state?: string
  at: number
  startedAt?: number
  durationSec?: number
  messages: number
  toolCalls: number
  toolErrors: number
  stuckTurns?: number
  tools?: DigestToolCount[]
  truncated?: boolean
  artifacts?: DigestRef[]
  notes?: DigestRef[]
  todo: DigestTodo
  children?: number
  childrenFailed?: number
  waitingAsk?: boolean
  lastError?: string
  openLoops?: string[]
  suggestNote?: boolean
  hash: string
  // The rendered digest, budgeted — shown verbatim.
  text: string
}

// One row of GET /api/awareness/digests (newest first).
export interface DigestIndexEntry {
  sessionId: string
  agentId?: string
  title: string
  at: number
  line: string
  hash: string
  openLoops?: number
  errors?: number
  artifacts?: number
  notes?: number
  suggestNote?: boolean
}

export type CompositionMoment = 'brief' | 'turn' | 'wrap'

export type CompositionSectionState = 'full' | 'pointer' | 'dropped' | 'cut'

export interface CompositionSection {
  key: string
  bytes: number
  state: CompositionSectionState
  priority: number
}

// The result of fitting awareness sections to a byte budget — what the agent
// was actually sent at one moment.
export interface Composition {
  moment: CompositionMoment
  text: string
  // The last line of text, kept separately for the UI.
  meter: string
  bytes: number
  budget: number
  sections: CompositionSection[]
  degraded?: string[]
  dropped?: string[]
  cut?: boolean
  hash: string
  at?: number
}

// GET /api/sessions/{id}/awareness — what this session's agent has seen.
export interface AwarenessSeen {
  sessionId: string
  brief?: Composition
  lastTurn?: Composition
  lastPulse?: string
  turns: number
  digest?: Digest
}
