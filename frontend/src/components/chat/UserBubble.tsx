import type { ReactNode } from 'react'
import type { Agent } from '../../types'
import { resolveColor } from '../../lib/avatar'

// MENTION_RE matches an "@token" the way the composer inserts mentions: "@" then
// non-space, non-"@" characters.
const MENTION_RE = /@[^\s@]+/g

const norm = (s: string) => s.toLowerCase().replace(/\s+/g, '')

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
export function UserBubble({ text, agents }: { text: string; agents: Agent[] }) {
  const hasMention = MENTION_RE.test(text)
  MENTION_RE.lastIndex = 0
  const isCommand = /^\/\S/.test(text.trim())

  if (isCommand) {
    return (
      <div className="flex justify-end">
        <div className="flex max-w-[80%] min-w-0 items-center gap-2 rounded-2xl border border-[var(--color-accent)]/60 bg-[var(--color-accent-soft)] px-4 py-2.5 font-mono text-sm break-words text-[var(--color-text)]">
          <span className="text-[var(--color-accent)]">⌘</span>
          <span className="min-w-0 break-words">{text}</span>
        </div>
      </div>
    )
  }

  return (
    <div className="flex justify-end">
      <div
        className={`max-w-[80%] min-w-0 whitespace-pre-wrap break-words rounded-2xl bg-[var(--color-accent)] px-4 py-3 text-sm leading-relaxed text-white ${
          hasMention ? 'ring-1 ring-white/40' : ''
        }`}
      >
        {hasMention ? renderWithMentions(text, agents) : text}
      </div>
    </div>
  )
}
