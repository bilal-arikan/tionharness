import type { Agent, Artifact, Message } from '../../types'
import { UserBubble } from './UserBubble'
import { MessageTime } from './MessageMeta'
import { DeleteButton } from './DeleteButton'
import { RewindButton } from './RewindButton'
import { DirectionBadge } from './DirectionBadge'

// UserTurn renders a real user message: the bubble plus a right-aligned meta row
// (rewind + delete on hover + timestamp). Auto-generated prompts use
// AutoPromptNote instead.
export function UserTurn({
  message,
  agents,
  artifacts,
  onDelete,
  onRewind,
  onOpenArtifact,
  clamp,
  recipientLabel,
}: {
  message: Message
  agents: Agent[]
  artifacts?: Artifact[]
  onDelete?: (id: string) => void
  // Rewind the conversation to this message (remove it + everything after).
  onRewind?: (id: string) => void
  onOpenArtifact?: (id: string) => void
  // Clamp the bubble text to 2 lines (used when this turn is pinned to the top).
  clamp?: boolean
  // "→ <name>" cue for the agent this message was routed to, in a multi-participant
  // thread (generic participant model). Undefined = single-participant / no cue.
  recipientLabel?: string
}) {
  const m = message
  return (
    <div className="group flex flex-col gap-1">
      <UserBubble
        text={m.text}
        agents={agents}
        attachments={m.attachments}
        artifacts={artifacts}
        onOpenArtifact={onOpenArtifact}
        clamp={clamp}
      />
      <div className="flex items-center justify-end gap-2 pr-1">
        <DirectionBadge label={recipientLabel} />
        {onRewind && <RewindButton onClick={() => onRewind(m.id)} />}
        {onDelete && <DeleteButton onClick={() => onDelete(m.id)} />}
        <MessageTime unixSec={m.createdAt} />
      </div>
    </div>
  )
}
