import type { Agent, Artifact, Message } from '../../types'
import { UserBubble } from './UserBubble'
import { MessageTime } from './MessageMeta'
import { DeleteButton } from './DeleteButton'

// UserTurn renders a real user message: the bubble plus a right-aligned meta row
// (delete-on-hover + timestamp). Auto-generated prompts use AutoPromptNote instead.
export function UserTurn({
  message,
  agents,
  artifacts,
  onDelete,
  onOpenArtifact,
}: {
  message: Message
  agents: Agent[]
  artifacts?: Artifact[]
  onDelete?: (id: string) => void
  onOpenArtifact?: (id: string) => void
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
      />
      <div className="flex items-center justify-end gap-2 pr-1">
        {onDelete && <DeleteButton onClick={() => onDelete(m.id)} />}
        <MessageTime unixSec={m.createdAt} />
      </div>
    </div>
  )
}
