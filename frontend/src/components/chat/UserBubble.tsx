import type { ReactNode } from 'react'
import type { Agent, Attachment } from '../../types'
import { resolveColor } from '../../lib/avatar'
import { AttachmentChip } from './AttachmentChip'

// MENTION_RE matches an "@token" the way the composer inserts mentions: "@" then
// non-space, non-"@" characters.
const MENTION_RE = /@[^\s@]+/g

const norm = (s: string) => s.toLowerCase().replace(/\s+/g, '')

// QUOTE_PAIRS maps an opening quote to its closing counterpart. Wrapping a
// command in any of these escapes it: the message is treated as plain prose
// (quotes stripped) instead of the command style — so you can talk *about* a
// command without it looking like one.
const QUOTE_PAIRS: Record<string, string> = {
  '"': '"',
  "'": "'",
  '`': '`',
  '“': '”', // “ ”
  '‘': '’', // ‘ ’
}

// quotedCommand returns the inner command text when `text` is a command (leading
// "/") wrapped in a matching quote pair, or null otherwise.
function quotedCommand(text: string): string | null {
  const t = text.trim()
  if (t.length < 3) return null
  const close = QUOTE_PAIRS[t[0]]
  if (!close || !t.endsWith(close)) return null
  const inner = t.slice(1, -1).trim()
  return /^\/\S/.test(inner) ? inner : null
}

// renderWithMentions splits a user message into plain text and highlighted
// @mention chips, colouring each chip with the mentioned agent's avatar colour
// when it resolves to a known agent.
function renderWithMentions(text: string, agents: Agent[]): ReactNode[] {
  const out: ReactNode[] = []
  let last = 0
  let m: RegExpExecArray | null
  MENTION_RE.lastIndex = 0
  while ((m = MENTION_RE.exec(text)) !== null) {
    if (m.index > last) out.push(text.slice(last, m.index))
    const raw = m[0]
    const q = norm(raw.slice(1))
    const ag =
      agents.find((a) => norm(a.name) === q) ??
      agents.find((a) => norm(a.name).startsWith(q))
    const color = ag ? resolveColor(ag) : undefined
    out.push(
      <span
        key={`${m.index}-${raw}`}
        className="mx-0.5 rounded px-1 font-semibold"
        style={color ? { backgroundColor: color, color: '#fff' } : { backgroundColor: 'rgba(255,255,255,0.28)' }}
      >
        {raw}
      </span>,
    )
    last = m.index + raw.length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

// UserBubble renders a user chat message. Plain messages keep the accent bubble;
// messages that mention agents (@) get highlighted chips + a ring, and messages
// typed as a command (leading "/") render in a distinct monospaced command style.
export function UserBubble({
  text,
  agents,
  attachments,
  onOpenArtifact,
}: {
  text: string
  agents: Agent[]
  attachments?: Attachment[]
  onOpenArtifact?: (id: string) => void
}) {
  // A quote-wrapped command is an explicit escape → render the inner text as a
  // plain bubble (no command style).
  const quotedCmd = quotedCommand(text)
  const hasMention = MENTION_RE.test(text)
  MENTION_RE.lastIndex = 0
  const isCommand = !quotedCmd && /^\/\S/.test(text.trim())

  // Attachment chips rendered under the bubble (image thumbnails / file cards).
  // Artifact-sourced chips are clickable: id format is "art-<artifactId>".
  const chips = attachments && attachments.length > 0 && (
    <div className="mt-1.5 flex flex-wrap justify-end gap-2">
      {attachments.map((a) => {
        const artifactClick =
          a.source === 'artifact' && onOpenArtifact
            ? () => onOpenArtifact(a.id.startsWith('art-') ? a.id.slice(4) : a.id)
            : undefined
        return <AttachmentChip key={a.id} attachment={a} onClick={artifactClick} />
      })}
    </div>
  )

  if (isCommand) {
    return (
      <div className="flex flex-col items-end">
        <div className="flex max-w-[80%] min-w-0 items-center gap-2 rounded-2xl border border-[var(--color-accent)]/60 bg-[var(--color-accent-soft)] px-4 py-2.5 font-mono text-sm break-words text-[var(--color-text)]">
          <span className="text-[var(--color-accent)]">⌘</span>
          <span className="min-w-0 break-words">{text}</span>
        </div>
        {chips}
      </div>
    )
  }

  return (
    <div className="flex flex-col items-end">
      {text.trim() && (
        <div
          className={`max-w-[80%] min-w-0 whitespace-pre-wrap break-words rounded-2xl bg-[var(--color-accent)] px-4 py-3 text-sm leading-relaxed text-white ${
            hasMention ? 'ring-1 ring-white/40' : ''
          }`}
        >
          {quotedCmd ? quotedCmd : hasMention ? renderWithMentions(text, agents) : text}
        </div>
      )}
      {chips}
    </div>
  )
}
