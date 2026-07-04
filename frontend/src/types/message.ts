// Chat messages, the assistant activity trace (TurnStep) and chat responses.
import type { Attachment } from './attachment'

// A single entry in an assistant turn's activity trace (mirrors agent.TurnStep).
// Transient (live-only): 'delta' (streaming text), 'ask' (interactive prompt),
// 'permission' (approval gate), 'plan' (plan approval), 'tool_delta' (streaming
// tool output), 'tombstone' (retract a live step).
// Persisted: text, thinking, tool, todo, recovery, error, steer.
// See lib/stepKinds.ts for human-readable descriptions.
export type StepKind =
  | 'text'
  | 'thinking'
  | 'tool'
  | 'delta'
  | 'ask'
  | 'permission'
  | 'plan'
  | 'todo'
  | 'recovery'
  | 'error'
  | 'steer'
  | 'tool_delta'
  | 'tombstone'
  | 'diff'
  | 'hook'
  | 'subagent'

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
  // Nested activity trace of a 'subagent' step — the subagent's own tool calls /
  // thinking, captured in its isolated context.
  subSteps?: TurnStep[]
}

// A slash command surfaced in the chat composer ("/" menu).
export interface SlashCommand {
  name: string // without the leading slash, e.g. "new"
  description: string
  icon?: string
  // run receives the text typed after the command name when takesInput is set
  // (e.g. "/myflow some topic" → run("some topic")), plus any attachments staged
  // in the composer; otherwise called with none.
  run: (input?: string, attachments?: Attachment[]) => void
  // takesInput: selecting from the menu inserts "/name " and waits for the user
  // to type an argument + Enter, rather than running immediately. Used by flows.
  takesInput?: boolean
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
  // True when this assistant reply was reconstructed from a crash sidecar (the
  // server died mid-stream): text/trace are partial and the UI flags it as cut
  // off. See backend db.InflightTurn / recoverInflight.
  interrupted?: boolean
  // True when the USER stopped this assistant turn mid-stream (distinct from
  // interrupted, which is a crash-recovered partial). Text/trace are whatever
  // completed before the stop.
  cancelled?: boolean
  // Per-turn enrichment (assistant role): the model that actually answered, why
  // generation ended, this turn's token usage, and its wall-clock duration. All
  // optional/absent for user/system or older messages.
  model?: string
  stopReason?: string
  usage?: MessageUsage
  durationMs?: number
  // The user's rating of this assistant turn (👍/👎 + optional note). Absent = none.
  feedback?: MessageFeedback
  // How an auto-generated prompt was produced (display only): "wake" = a
  // schedule_wake auto-resume, "schedule" = a scheduled routine prompt. Empty for
  // real user messages. The UI renders these as a "⏰ continuation" note instead
  // of a user bubble so the agent doesn't look like it is asking itself.
  origin?: string
  reasoningContent?: string
  // User-supplied files / pasted long text sent with this turn (user role only).
  attachments?: Attachment[]
  createdAt: number
}

// InflightSnapshot is a session's in-progress streaming reply as written to the
// crash sidecar on a throttle (backend db.InflightTurn). Fetched after a mid-turn
// reload to restore the partial assistant bubble (agent + steps-so-far) while the
// detached turn keeps running. `steps` is JSON-encoded TurnStep[] (may be "[]").
export interface InflightSnapshot {
  messageId: string
  sessionId: string
  agentId: string
  startedAt: number
  text: string
  steps: string
}

// MessageUsage is an assistant turn's token consumption (compact keys mirror the
// backend db.MessageUsage).
export interface MessageUsage {
  in: number
  out: number
  cacheRead?: number
  cacheWrite?: number
}

// MessageFeedback is the user's rating of an assistant turn.
export interface MessageFeedback {
  rating: number // +1 | -1 | 0
  note?: string
  at: number
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
