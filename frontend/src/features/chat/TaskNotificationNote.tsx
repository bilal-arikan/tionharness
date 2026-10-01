import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { formatDurationMs } from '@/shared/lib/time'
import { Bot, CheckCircle2, ChevronRight, Clock, OctagonX, Wrench, XCircle } from 'lucide-react'
import type { Message } from '@/types'
import { MessageTime } from './MessageMeta'
import { DeleteButton } from './DeleteButton'
import { notificationChanges, parseTaskNotification } from './parseTaskNotification'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { AgentIdentity, type AgentLike } from '@/shared/components/agents/AgentIdentity'
import { DiffCard } from './DiffCard'
import { ExecutionNotice } from './ExecutionNotice'
import { notificationExecutionSteps, notificationTraceInvalid } from './executionEvidence'

// TaskNotificationNote renders a coordinator's <task-notification> injection
// (Message.origin === "worker-note") as a compact worker-result card instead of
// dumping the raw XML: a header with the worker agent, status badge, session id
// and usage, plus the result body folded by default. The raw text stays intact
// in the DB — this is display-only parsing.

const STATUS_META: Record<string, { cls: string; Icon: typeof CheckCircle2 }> = {
  completed: { cls: 'text-[var(--color-success)]', Icon: CheckCircle2 },
  failed: { cls: 'text-[var(--color-danger)]', Icon: XCircle },
  killed: { cls: 'text-[var(--color-warning)]', Icon: OctagonX },
  timeout: { cls: 'text-[var(--color-warning)]', Icon: Clock },
  incomplete: { cls: 'text-[var(--color-warning)]', Icon: Bot },
}

// fmtDuration parses the notification's millisecond field (a string in the
// payload) and renders it with the shared duration formatter.
function fmtDuration(ms: string): string {
  const n = Number(ms)
  if (!Number.isFinite(n) || n <= 0) return ''
  return formatDurationMs(n)
}

export function TaskNotificationNote({
  message,
  agent,
  onSelectSession,
  onOpenFile,
  onDelete,
  laterStatus,
}: {
  message: Message
  agent?: AgentLike
  onSelectSession?: (id: string) => void
  onOpenFile?: (path: string) => void
  onDelete?: (id: string) => void
  laterStatus?: string
}) {
  const { t } = useTranslation('chatStatus')
  const [open, setOpen] = useState(false)
  const p = parseTaskNotification(message.text)
  // Unparseable worker-note (foreign/legacy format): show the raw text folded so
  // nothing is silently hidden.
  const statusMeta = p
    ? (STATUS_META[p.status] ?? { cls: 'text-[var(--color-text-dim)]', Icon: Bot })
    : null
  const statusLabel = p
    ? STATUS_META[p.status]
      ? t(`taskNotification.status.${p.status}`)
      : p.status
    : ''
  const body = p ? p.result || p.summary : message.text
  const duration = p ? fmtDuration(p.durationMs) : ''
  const identity = p
    ? {
        ...(agent ?? { id: p.agentId, name: p.agent || p.agentId }),
        model: p.model || agent?.model,
      }
    : undefined
  const traceInvalid = notificationTraceInvalid(message.steps)
  const changes = traceInvalid ? [] : notificationChanges(message.steps)
  const executionSteps = notificationExecutionSteps(message.steps)

  return (
    <div className="group flex flex-col items-center gap-1" data-testid="task-notification">
      <div className="w-full max-w-[85%] rounded-xl border border-[color-mix(in_srgb,var(--color-accent)_30%,var(--color-border))] bg-[var(--color-accent-soft)] text-xs">
        <div className="flex w-full items-center gap-2 px-3 py-2">
          <button
            type="button"
            onClick={() => setOpen((o) => !o)}
            aria-expanded={open}
            aria-label={open ? t('taskNotification.collapse') : t('taskNotification.expand')}
            className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
          >
            <ChevronRight size={13} className={`transition-transform ${open ? 'rotate-90' : ''}`} />
          </button>
          {identity ? (
            p?.taskId && onSelectSession ? (
              <button
                type="button"
                onClick={() => onSelectSession(p.taskId)}
                aria-label={t('taskNotification.openSession', { name: identity.name })}
                className="inline-flex min-w-0 flex-1 items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-left text-[var(--color-text)] transition hover:border-[var(--color-accent)]"
              >
                <AgentIdentity
                  agent={identity}
                  size="sm"
                  showId={Boolean(identity.id)}
                  subtitle={
                    identity.provider
                      ? t('taskNotification.model')
                      : identity.model || t('taskNotification.none')
                  }
                />
              </button>
            ) : (
              <AgentIdentity
                agent={identity}
                size="sm"
                showId={Boolean(identity.id)}
                subtitle={
                  identity.provider
                    ? t('taskNotification.model')
                    : identity.model || t('taskNotification.none')
                }
                className="min-w-0 flex-1"
              />
            )
          ) : (
            <span className="min-w-0 flex-1 truncate font-medium text-[var(--color-accent)]">
              {t('taskNotification.notification')}
            </span>
          )}
          {p && statusMeta && (
            <div
              className="flex shrink-0 flex-col items-end gap-0.5"
              data-testid="task-status-meta"
            >
              <span className={`flex items-center gap-1 font-medium ${statusMeta.cls}`}>
                <statusMeta.Icon size={13} /> {statusLabel}
              </span>
              {(p.toolUses || duration) && (
                <span className="flex items-center justify-end gap-3 text-[10px] text-[var(--color-text-dim)]">
                  {p.toolUses && (
                    <span className="flex items-center gap-1">
                      <Wrench size={10} />{' '}
                      {t('taskNotification.toolCount', { count: Number(p.toolUses) })}
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
        {laterStatus && (
          <div
            className="border-t border-[var(--color-border)] px-3 py-1.5 text-[10px] text-[var(--color-warning)]"
            data-testid="task-later-report"
          >
            {t('taskNotification.laterReport', {
              status: t(`taskNotification.status.${laterStatus}`, { defaultValue: laterStatus }),
            })}
          </div>
        )}
        {p?.status === 'completed' && (
          <div
            className="border-t border-[var(--color-border)] px-3 py-1.5 text-[10px] text-[var(--color-text-dim)]"
            data-testid="task-report-evidence"
          >
            {t(
              p.source === 'runtime'
                ? 'taskNotification.runtimeOutcome'
                : 'taskNotification.reportedOutcome',
            )}
          </div>
        )}
        {traceInvalid && (
          <p
            role="status"
            className="border-t border-[var(--color-border)] px-3 py-2 text-[var(--color-warning)]"
          >
            {t('executionNotice.traceUnavailable')}
          </p>
        )}
        {executionSteps.length > 0 && (
          <div className="border-t border-[var(--color-border)] px-3 py-1">
            {executionSteps.map((step, index) => (
              <ExecutionNotice
                key={step.id ?? `${step.operation}-${index}`}
                step={step}
                onOpenFile={onOpenFile}
              />
            ))}
          </div>
        )}
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
