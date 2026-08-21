// Chat messages, the assistant activity trace (TurnStep) and chat responses.
import type { Attachment } from './attachment'

// A single entry in an assistant turn's activity trace (mirrors agent.TurnStep).
// Transient (live-only): 'delta' (streaming text), 'ask' (interactive prompt),
// 'permission' (approval gate), 'plan' (plan approval), 'tool_delta' (streaming
// tool output), 'tombstone' (retract a live step).
// Persisted: text, thinking, tool, todo, recovery, error, steer.
// See @/shared/stepKinds for human-readable descriptions + shared icons.
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
  | 'context_change'
  | 'cache_break'

export interface TodoItem {
  content: string
  status: 'pending' | 'in_progress' | 'completed'
}

// One changed region of the frozen static context (prompt-epoch drift), self-
// labelled by the first line of the changed paragraph. kind ∈ added | removed.
export interface ContextArea {
  label: string
  kind: 'added' | 'removed'
  lines?: string[]
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
  // Multiple questions for a single 'ask' prompt — each with its own optional
  // options. When set, the UI renders them together in one card, answered at once.
  questions?: { question: string; options?: string[] }[]
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
  // Marks a PARTIAL live 'subagent' card: the delegation is still running and a
  // later step with the same id replaces this one. Never set on a persisted step.
  running?: boolean
  // Parallel-batch group id (1-based, unique within the turn): steps born from
  // ONE provider response that carried multiple parallel tool calls share it, so
  // the UI clusters them. Absent/0 = lone call.
  batch?: number
  // 'context_change' payload: the per-block added/removed diff of the frozen
  // static context (prompt-epoch drift). added/removed above hold rollup counts.
  areas?: ContextArea[]
  // 'cache_break' payload: the prefix size (tokens) this turn had to re-pay cold
  // after losing the warm prompt cache. `reason` carries the attributed cause.
  coldTokens?: number
  // Truncation markers set by the SERVER on the transcript READ path only: a
  // long tool payload is cut to a cap so opening a session does not ship
  // megabytes the chat never paints. The persisted trace — and the model's
  // context — keeps the full text; sessionApi.getMessageSteps refetches it on
  // demand. `*Len` carries the original byte length.
  outputTruncated?: boolean
  outputLen?: number
  textTruncated?: boolean
  textLen?: number
  patchTruncated?: boolean
  patchLen?: number
  inputTruncated?: boolean
  // Set when an external token-optimizer shrank this shell step's output before
  // it re-entered the model's context, so the card can show a chip rather than
  // the rewrite being invisible. 'sqz' carries real token counts; 'rtk' wrapped
  // the command upstream, so it has no before/after pair to report.
  optimizer?: ShellOptimization
}

export interface ShellOptimization {
  kind: 'sqz' | 'rtk'
  inTokens?: number
  outTokens?: number
  // sqz recognised output identical to an earlier result this session and replaced
  // the WHOLE thing with a `§ref:<hash>§` pointer — so a command that printed
  // thousands of lines shows one line here. Flagged so the card can say why.
  dedup?: boolean
  // The rewritten command, when an optimizer changed what actually ran (rtk turns
  // `go test -v ./...` into `go test -json ./...`). Surfaced so a command
  // substitution never happens behind the user's back.
  command?: string
  // A rewritten command that FAILED: the output is rtk's summary, which can lose
  // the real error. The card warns instead of presenting it as a clean result.
  degraded?: boolean
}

// A slash command surfaced in the chat composer ("/" menu).
export interface SlashCommand {
  name: string // without the leading slash, e.g. "new"
  description: string
  icon?: string
  // run receives the text typed after the command name when takesInput is set
  // (e.g. "/myflow some topic" → run("some topic")), plus any attachments staged
  // in the composer; otherwise called with none.
  // A command may run asynchronously: the composer awaits the result and keeps
  // the typed input when it resolves to false (the command failed), so the user
  // can retry without retyping. Returning void/undefined clears as before.
  run: (input?: string, attachments?: Attachment[]) => void | Promise<boolean | void>
  // takesInput: selecting from the menu inserts "/name " and waits for the user
  // to type an argument + Enter, rather than running immediately. Used by flows.
  takesInput?: boolean
}

// SessionChangeStep is one file-mutating trace step lifted out of a session's
// whole transcript by GET /api/sessions/{id}/changes, tagged with the turn it
// came from. Unlike the transcript listing, its `step` is UNTRIMMED — the bulk
// changes popup needs the full patch. Everything that is not a file mutation is
// dropped server-side, which is what keeps the call cheap.
export interface SessionChangeStep {
  msgId: string
  agentId?: string
  createdAt: number
  // The change was made by a subagent nested inside the turn.
  nested?: boolean
  step: TurnStep
}

export interface Message {
  id: string
  sessionId: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  // Which agent produced an assistant turn (empty for user/system). Multi-agent
  // sessions tag each turn so the UI can show the responding agent's avatar. On a
  // user turn it carries the legacy "routed recipient agent" (see recipientId).
  agentId?: string
  // Generic participant model (db.Message): who wrote this turn and, when directed,
  // whom it addresses. authorKind ∈ "user" | "agent" | "system"; authorId is the
  // agent id or "user"; recipientId is an agent id, "*" (broadcast), or empty
  // (thread at large). The UI renders a "→ <name>" direction cue from recipientId.
  authorKind?: 'user' | 'agent' | 'system'
  authorId?: string
  recipientId?: string
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

// BtwResponse is one side-chat ("btw") answer. It carries no message ids because
// no message was created: the side chat reads the session's context but is never
// written back into its history.
export interface BtwResponse {
  answer: string
  model: string
  usage: Usage
}
