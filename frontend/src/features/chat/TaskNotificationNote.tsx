import { useState } from 'react'
import { Bot, CheckCircle2, ChevronRight, Clock, OctagonX, Wrench, XCircle } from 'lucide-react'
import type { Message } from '@/types'
import { MessageTime } from './MessageMeta'
import { DeleteButton } from './DeleteButton'
import { parseTaskNotification } from './parseTaskNotification'

// TaskNotificationNote renders a coordinator's <task-notification> injection
// (Message.origin === "worker-note") as a compact worker-result card instead of
// dumping the raw XML: a header with the worker agent, status badge, session id
// and usage, plus the result body folded by default. The raw text stays intact
// in the DB — this is display-only parsing.

const STATUS_META: Record<string, { label: string; cls: string; Icon: typeof CheckCircle2 }> = {
  completed: { label: 'tamamlandı', cls: 'text-[var(--color-success)]', Icon: CheckCircle2 },
  failed: { label: 'başarısız', cls: 'text-[var(--color-danger)]', Icon: XCircle },
  killed: { label: 'durduruldu', cls: 'text-[var(--color-warning)]', Icon: OctagonX },
}

function fmtDuration(ms: string): string {
  const n = Number(ms)
  if (!Number.isFinite(n) || n <= 0) return ''
  return n >= 1000 ? `${Math.round(n / 1000)} sn` : `${n} ms`
}

export function TaskNotificationNote({
  message,
  onDelete,
}: {
  message: Message
  onDelete?: (id: string) => void
}) {
  const [open, setOpen] = useState(false)
  const p = parseTaskNotification(message.text)
  // Unparseable worker-note (foreign/legacy format): show the raw text folded so
  // nothing is silently hidden.
  const status = p
    ? (STATUS_META[p.status] ?? { label: p.status, cls: 'text-[var(--color-text-dim)]', Icon: Bot })
    : null
  const body = p ? p.result || p.summary : message.text
  const duration = p ? fmtDuration(p.durationMs) : ''

  return (
    <div className="group flex flex-col items-center gap-1" data-testid="task-notification">
      <div className="w-full max-w-[85%] rounded-xl border border-[color-mix(in_srgb,var(--color-accent)_30%,var(--color-border))] bg-[var(--color-accent-soft)] text-xs">
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
          className="flex w-full items-center gap-2 px-3 py-2 text-left"
        >
          <ChevronRight
            size={13}
            className={`shrink-0 text-[var(--color-text-dim)] transition-transform ${open ? 'rotate-90' : ''}`}
          />
          <Bot size={14} className="shrink-0 text-[var(--color-accent)]" />
          <span className="min-w-0 flex-1 truncate">
            <span className="font-medium text-[var(--color-accent)]">Worker bildirimi</span>
            {p?.agent && <span className="text-[var(--color-text)]"> — {p.agent}</span>}
            {p?.taskId && <span className="text-[var(--color-text-dim)]"> ({p.taskId})</span>}
          </span>
          {p && status && (
            <span className={`flex shrink-0 items-center gap-1 font-medium ${status.cls}`}>
              <status.Icon size={13} /> {status.label}
            </span>
          )}
        </button>
        {p && (p.toolUses || duration) && (
          <div className="flex items-center gap-3 px-3 pb-1 pl-9 text-[10px] text-[var(--color-text-dim)]">
            {p.toolUses && (
              <span className="flex items-center gap-1">
                <Wrench size={10} /> {p.toolUses} araç
              </span>
            )}
            {duration && (
              <span className="flex items-center gap-1">
                <Clock size={10} /> {duration}
              </span>
            )}
          </div>
        )}
        {open && (
          <div className="max-h-80 overflow-y-auto border-t border-[color-mix(in_srgb,var(--color-accent)_20%,var(--color-border))] px-3 py-2">
            <pre className="whitespace-pre-wrap break-words font-sans text-[11px] leading-relaxed text-[var(--color-text)]">
              {body}
            </pre>
          </div>
        )}
      </div>
      <div className="flex items-center gap-2">
        {onDelete && <DeleteButton onClick={() => onDelete(message.id)} />}
        <MessageTime unixSec={message.createdAt} />
      </div>
    </div>
  )
}
