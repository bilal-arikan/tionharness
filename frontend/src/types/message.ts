// Chat messages, the assistant activity trace (TurnStep) and chat responses.
import type { Attachment } from './attachment'

// A single entry in an assistant turn's activity trace (mirrors agent.TurnStep).
// Transient (live-only): 'delta' (streaming text), 'ask' (interactive prompt),
// 'tool_delta' (streaming tool output), 'tombstone' (retract a live step).
// Persisted: text, thinking, tool, todo, recovery, error, steer.
// See lib/stepKinds.ts for human-readable descriptions.
export type StepKind =
  | 'text'
  | 'thinking'
  | 'tool'
  | 'delta'
  | 'ask'
  | 'todo'
  | 'recovery'
  | 'error'
  | 'steer'
  | 'tool_delta'
  | 'tombstone'
  | 'diff'

export interface TodoItem {
  content: string
  status: 'pending' | 'in_progress' | 'completed'
}

export interface TurnStep {
  kind: StepKind
  text?: string
  tool?: string
  input?: unknown
  output?: string
  isError?: boolean
  // Suggested clickable answers for an 'ask' prompt.
  options?: string[]
  // Checklist items for a 'todo' step.
  todos?: TodoItem[]
  // Stable machine tag for a 'recovery'/'error' step (e.g. "max_tool_iterations").
  reason?: string
  // Optional id of a live step, referenced by a 'tombstone' or 'tool_delta'.
  id?: string
  // Target step id a 'tombstone' retracts.
  ref?: string
  // 'diff' step payload: changed file path, added/removed line counts, an
  // optional unified patch, and whether the file was newly created.
  path?: string
  added?: number
  removed?: number
  patch?: string
  created?: boolean
}

// A slash command surfaced in the chat composer ("/" menu).
export interface SlashCommand {
  name: string // without the leading slash, e.g. "new"
  description: string
  icon?: string
  run: () => void
}

export interface Message {
  id: string
  sessionId: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  // Which agent produced an assistant turn (empty for user/system). Multi-agent
  // sessions tag each turn so the UI can show the responding agent's avatar.
  agentId?: string
  text: string
  // JSON-encoded TurnStep[] as persisted by the backend (empty "[]" for plain
  // replies). Parsed lazily by the renderer.
  steps?: string
  reasoningContent?: string
  // User-supplied files / pasted long text sent with this turn (user role only).
  attachments?: Attachment[]
  createdAt: number
}

export interface Usage {
  inputTokens: number
  outputTokens: number
}

export interface ChatResponse {
  reply: string
  usage: Usage
  model: string
  userMessage: Message
  replyMessage: Message
  // Live activity trace for this turn (tool calls + intermediate text).
  steps?: TurnStep[]
  // Present only when the first turn auto-generated the session title.
  sessionTitle?: string
}
