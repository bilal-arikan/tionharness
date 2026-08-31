import { useState } from 'react'
import { Bot, CheckCircle2, ChevronRight, Clock, OctagonX, Wrench, XCircle } from 'lucide-react'
import type { Message } from '@/types'
import { MessageTime } from './MessageMeta'
import { DeleteButton } from './DeleteButton'
import { notificationChanges, parseTaskNotification } from './parseTaskNotification'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { AgentIdentity, type AgentLike } from '@/shared/components/agents/AgentIdentity'
import { DiffCard } from './DiffCard'

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
  agent,
  onSelectSession,
  onOpenFile,
  onDelete,
}: {
  message: Message
  agent?: AgentLike
  onSelectSession?: (id: string) => void
  onOpenFile?: (path: string) => void
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
  const identity = p
    ? {
        ...(agent ?? { id: p.agentId, name: p.agent || p.agentId }),
        model: p.model || agent?.model,
      }
    : undefined
  const changes = notificationChanges(message.steps)

  return (
    <div className="group flex flex-col items-center gap-1" data-testid="task-notification">
      <div className="w-full max-w-[85%] rounded-xl border border-[color-mix(in_srgb,var(--color-accent)_30%,var(--color-border))] bg-[var(--color-accent-soft)] text-xs">
        <div className="flex w-full items-center gap-2 px-3 py-2">
          <button
            type="button"
            onClick={() => setOpen((o) => !o)}
            aria-expanded={open}
            aria-label={open ? 'Worker sonucunu daralt' : 'Worker sonucunu genişlet'}
            className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
          >
            <ChevronRight size={13} className={`transition-transform ${open ? 'rotate-90' : ''}`} />
          </button>
          {identity ? (
            p?.taskId && onSelectSession ? (
              <button
                type="button"
                onClick={() => onSelectSession(p.taskId)}
                aria-label={`${identity.name} worker oturumunu aç`}
                className="inline-flex min-w-0 flex-1 items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-left text-[var(--color-text)] transition hover:border-[var(--color-accent)]"
              >
                <AgentIdentity
                  agent={identity}
                  size="sm"
                  showId={Boolean(identity.id)}
                  subtitle={identity.provider ? 'model' : identity.model || 'none'}
                />
              </button>
            ) : (
              <AgentIdentity
                agent={identity}
                size="sm"
                showId={Boolean(identity.id)}
                subtitle={identity.provider ? 'model' : identity.model || 'none'}
                className="min-w-0 flex-1"
              />
            )
          ) : (
            <span className="min-w-0 flex-1 truncate font-medium text-[var(--color-accent)]">
              Worker bildirimi
            </span>
          )}
          {p && status && (
            <div
              className="flex shrink-0 flex-col items-end gap-0.5"
              data-testid="task-status-meta"
            >
              <span className={`flex items-center gap-1 font-medium ${status.cls}`}>
                <status.Icon size={13} /> {status.label}
              </span>
              {(p.toolUses || duration) && (
                <span className="flex items-center justify-end gap-3 text-[10px] text-[var(--color-text-dim)]">
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
                </span>
              )}
            </div>
          )}
        </div>
        {open && (
          <div className="max-h-80 space-y-2 overflow-y-auto border-t border-[color-mix(in_srgb,var(--color-accent)_20%,var(--color-border))] px-3 py-2 text-[11px] leading-relaxed text-[var(--color-text)]">
            <Markdown>{body}</Markdown>
            {changes.map((change, index) => (
              <DiffCard key={`${change.path}-${index}`} step={change} onOpenFile={onOpenFile} />
            ))}
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
