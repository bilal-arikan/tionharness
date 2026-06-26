import { useState, type ReactNode } from 'react'
import type { Agent, Artifact, Attachment } from '../../types'
import { resolveColor } from '../../lib/avatar'
import { AttachmentChip } from './AttachmentChip'
import { Lightbox } from '../common'
import { imageURL } from '../../lib/attachments'

// MENTION_RE matches an "@token" the way the composer inserts a name reference:
// "@" then non-space, non-"@" characters. A mention is a plain reference to an
// agent by name (highlighted as a chip) — it does NOT route the turn.
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
// @mention chips, colouring each chip with the referenced agent's avatar colour
// when it resolves to a known agent. The chip is a visual NAME REFERENCE only.
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

// Attachment images open in the shared Lightbox (zoom + pan + Escape/backdrop).


// UserBubble renders a user chat message. Plain messages keep the accent bubble;
// messages that reference agents (@) get highlighted name chips + a ring, and
// messages typed as a command (leading "/") render in a distinct monospaced
// command style.
export function UserBubble({
  text,
  agents,
  attachments,
  artifacts,
  onOpenArtifact,
}: {
  text: string
  agents: Agent[]
  attachments?: Attachment[]
  artifacts?: Artifact[]
  onOpenArtifact?: (id: string) => void
}) {
  const [lightbox, setLightbox] = useState<{ url: string; name: string } | null>(null)

  // A quote-wrapped command is an explicit escape → render the inner text as a
  // plain bubble (no command style).
  const quotedCmd = quotedCommand(text)
  const hasMention = MENTION_RE.test(text)
  MENTION_RE.lastIndex = 0
  const isCommand = !quotedCmd && /^\/\S/.test(text.trim())

  // Every chat attachment is captured server-side as a session artifact (origin
  // "chat", keyed by sourcePath === relPath). Clicking a chip opens that artifact
  // in the viewer. Fallbacks: artifact-sourced chips (added via "#") carry the id
  // directly; an image not yet captured opens in an in-app lightbox.
  const chips = attachments && attachments.length > 0 && (
    <div className="mt-1.5 flex flex-wrap justify-end gap-2">
      {attachments.map((a) => {
        let onClick: (() => void) | undefined
        if (a.source === 'artifact' && onOpenArtifact) {
          onClick = () => onOpenArtifact(a.id.startsWith('art-') ? a.id.slice(4) : a.id)
        } else {
          const art = a.relPath ? artifacts?.find((x) => x.sourcePath === a.relPath) : undefined
          if (art && onOpenArtifact) {
            onClick = () => onOpenArtifact(art.id)
          } else if (a.kind === 'image') {
            const url = imageURL(a)
            if (url) onClick = () => setLightbox({ url, name: a.name })
          }
        }
        return <AttachmentChip key={a.id} attachment={a} onClick={onClick} />
      })}
    </div>
  )

  if (isCommand) {
    return (
      <>
        {lightbox && <Lightbox imageSrc={lightbox.url} imageAlt={lightbox.name} title={lightbox.name} onClose={() => setLightbox(null)} />}
        <div className="flex flex-col items-end">
          <div className="flex max-w-[80%] min-w-0 items-center gap-2 rounded-2xl border border-[var(--color-accent)]/60 bg-[var(--color-accent-soft)] px-4 py-2.5 font-mono text-sm break-words text-[var(--color-text)]">
            <span className="text-[var(--color-accent)]">⌘</span>
            <span className="min-w-0 break-words">{text}</span>
          </div>
          {chips}
        </div>
      </>
    )
  }

  return (
    <>
      {lightbox && <Lightbox imageSrc={lightbox.url} imageAlt={lightbox.name} title={lightbox.name} onClose={() => setLightbox(null)} />}
      <div className="flex flex-col items-end">
        {text.trim() && (
          <div
            className={`max-w-[80%] min-w-0 whitespace-pre-wrap break-words rounded-2xl bg-[color-mix(in_srgb,var(--color-accent)_82%,black)] px-4 py-3 text-sm leading-relaxed text-white ${
              hasMention ? 'ring-1 ring-white/40' : ''
            }`}
          >
            {quotedCmd ? quotedCmd : hasMention ? renderWithMentions(text, agents) : text}
          </div>
        )}
        {chips}
      </div>
    </>
  )
}
