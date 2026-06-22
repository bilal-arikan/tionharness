import { AlarmClock } from 'lucide-react'
import type { Message } from '../../types'
import { MessageTime } from './MessageMeta'
import { DeleteButton } from './DeleteButton'

// AutoPromptNote renders an auto-generated prompt (schedule_wake resume / scheduled
// routine) as a centered "⏰ continuation" note instead of a user bubble — the
// agent resumed itself; the user did not re-ask. Keyed off Message.origin.
export function AutoPromptNote({
  message,
  onDelete,
}: {
  message: Message
  onDelete?: (id: string) => void
}) {
  const m = message
  return (
    <div className="group flex flex-col items-center gap-1">
      <div className="flex max-w-[85%] items-start gap-2 rounded-xl border border-[color-mix(in_srgb,var(--color-accent)_30%,var(--color-border))] bg-[var(--color-accent-soft)] px-3 py-2 text-xs">
        <AlarmClock size={14} className="mt-0.5 shrink-0 text-[var(--color-accent)]" />
        <span className="min-w-0">
          <span className="font-medium text-[var(--color-accent)]">
            {m.origin === 'schedule' ? 'Zamanlanmış görev' : 'Otomatik devam'}
          </span>
          {m.text.trim() && <span className="text-[var(--color-text-dim)]"> — {m.text}</span>}
        </span>
      </div>
      <div className="flex items-center gap-2">
        {onDelete && <DeleteButton onClick={() => onDelete(m.id)} />}
        <MessageTime unixSec={m.createdAt} />
      </div>
    </div>
  )
}
