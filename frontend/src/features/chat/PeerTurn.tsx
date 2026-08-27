import { memo } from 'react'
import type { Message } from '@/types'
import type { AgentLike } from '@/shared/components/agents/AgentIdentity'
import { AgentHeader } from './AgentHeader'
import { MessageTime } from './MessageMeta'
import { DeleteButton } from './DeleteButton'
import { DirectionBadge } from './DirectionBadge'

// PeerTurn renders an INCOMING peer message: another agent wrote it and it landed
// in this agent's inbox. Although it is stored with role "user" (providers accept
// only the system/user/assistant trichotomy, so a peer message cannot role-flip),
// it is authored by an agent — so we render it LEFT-aligned with the SENDER's
// avatar + name, like an incoming message from someone else, instead of the
// right-aligned accent bubble reserved for the human's own turns. A "→ <recipient>"
// cue below states whom it was addressed to (the inbox owner in a DM). Falls back
// to the raw author id when the sender agent is not in the roster, so attribution
// is never silently dropped.
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
        <div className="max-w-[80%] min-w-0 rounded-2xl border border-[var(--color-sender-bubble-border)] bg-[var(--color-sender-bubble)] px-4 py-3 text-[var(--color-on-sender-bubble)]">
          {sender ? (
            <AgentHeader agent={sender} onOpenAgent={onOpenAgent} />
          ) : m.authorId ? (
            <div className="mb-1.5 font-mono text-xs text-[var(--color-text-dim)]">
              {m.authorId}
            </div>
          ) : null}
          <div className="whitespace-pre-wrap break-words text-sm leading-relaxed text-[var(--color-on-sender-bubble)]">
            {m.text}
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
