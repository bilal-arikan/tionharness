import { memo } from 'react'
import type { Message } from '@/types'
import type { AgentLike } from '@/shared/components/agents/AgentIdentity'
import { AgentHeader } from './AgentHeader'
import { MessageTime } from './MessageMeta'
import { DeleteButton } from './DeleteButton'
import { DirectionBadge } from './DirectionBadge'
import { CollapsibleBody } from './CollapsibleBody'
import { plainTextToMarkdown } from './hardBreaks'
import { Markdown } from '@/shared/components/markdown/Markdown'

// PeerTurn renders an INCOMING agent-authored message: another agent wrote it and
// it landed in this transcript. Although it is stored with role "user" (providers
// accept only the system/user/assistant trichotomy, so such a message cannot
// role-flip),
// it is authored by an agent — so we render it LEFT-aligned with the SENDER's
// avatar + name, like an incoming message from someone else, instead of the
// right-aligned accent bubble reserved for the human's own turns. A "→ <recipient>"
// cue below states whom it was addressed to. Falls back
// to the raw author id when the sender agent is not in the roster, so attribution
// is never silently dropped. The body is rendered as markdown (briefs use lists,
// code spans and fences) with its plain line breaks kept, spans the column like
// an assistant card, and folds when it runs long.
export const PeerTurn = memo(function PeerTurn({
  message,
  sender,
  recipientLabel,
  onDelete,
  onOpenAgent,
}: {
  message: Message
  // The authoring agent (resolved from Message.authorId); undefined when unknown.
  sender?: AgentLike
  // "→ <name>" cue for the addressee (recipientId), or "herkes" for a broadcast.
  recipientLabel?: string
  onDelete?: (id: string) => void
  onOpenAgent?: (id: string) => void
}) {
  const m = message
  return (
    <div className="group flex flex-col gap-1">
      <div className="flex w-full justify-start">
        <div className="w-full min-w-0 rounded-2xl border border-[var(--color-sender-bubble-border)] bg-[var(--color-sender-bubble)] px-4 py-3 text-[var(--color-on-sender-bubble)]">
          {sender ? (
            <AgentHeader agent={sender} onOpenAgent={onOpenAgent} />
          ) : m.authorId ? (
            <div className="mb-1.5 font-mono text-xs text-[var(--color-text-dim)]">
              {m.authorId}
            </div>
          ) : null}
          <div className="break-words text-[var(--color-on-sender-bubble)]">
            <CollapsibleBody>
              <Markdown>{plainTextToMarkdown(m.text)}</Markdown>
            </CollapsibleBody>
          </div>
        </div>
      </div>
      <div className="flex items-center gap-2 pl-1">
        <DirectionBadge label={recipientLabel} />
        {onDelete && <DeleteButton onClick={() => onDelete(m.id)} />}
        <MessageTime unixSec={m.createdAt} />
      </div>
    </div>
  )
})
