import { memo } from 'react'
import type { Agent, Artifact, Message } from '@/types'
import { UserBubble } from './UserBubble'
import { MessageTime } from './MessageMeta'
import { DeleteButton } from './DeleteButton'
import { RewindButton } from './RewindButton'
import { DirectionBadge } from './DirectionBadge'
import { ACTION_CLUSTER, META_CLUSTER, TURN_FOOTER_END } from './messageActions'

// UserTurn renders a real user message: the bubble, then a footer row BELOW it
// carrying the passive meta (timestamp + routing cue) and the rewind/delete
// controls. The user's bubble hugs the right edge, so the footer is right-aligned
// under it (a full-width justify-between would strand the meta on the far left,
// visually detached from the bubble). Auto-generated prompts use AutoPromptNote.
export const UserTurn = memo(function UserTurn({
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
      <div className={TURN_FOOTER_END}>
        <div className={META_CLUSTER}>
          <MessageTime unixSec={m.createdAt} />
          <DirectionBadge label={recipientLabel} />
        </div>
        {(onRewind || onDelete) && (
          <div className={ACTION_CLUSTER}>
            {onRewind && <RewindButton onClick={() => onRewind(m.id)} />}
            {onDelete && <DeleteButton onClick={() => onDelete(m.id)} />}
          </div>
        )}
      </div>
    </div>
  )
})
