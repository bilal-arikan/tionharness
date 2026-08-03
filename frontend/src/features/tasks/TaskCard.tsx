import { memo } from 'react'
import { Paperclip } from 'lucide-react'
import type { Agent, Flow, Task } from '@/types'
import { AgentIdentity } from '@/shared/components/agents/AgentIdentity'
import { normalizeAvatar } from '@/shared/lib/avatar'

// Priority chip colors/labels, keyed by the stored priority slug.
const PRIORITY_META: Record<string, { label: string; color: string }> = {
  critical: { label: 'Kritik', color: '#ef4444' },
  high: { label: 'Yüksek', color: '#f59e0b' },
  medium: { label: 'Orta', color: '#3b82f6' },
  low: { label: 'Düşük', color: '#6b7280' },
}

// TaskCardMeta is everything the card shows that is derived rather than stored on
// the task. The board computes it once per data change (id-indexed) so no card
// ever runs a lookup during render.
export interface TaskCardMeta {
  owner: Agent | undefined
  flow: Flow | undefined
  depIds: string[]
  unmetDeps: string[]
  unmetColColor: string | null
}

interface Props {
  task: Task
  meta: TaskCardMeta
  selected: boolean
  /** An OS file drag is hovering THIS card (ring highlight). */
  fileDropActive: boolean
  /** Today in ISO, for the due-date chip. Passed in so every card agrees. */
  today: string
  onDragStart: (taskId: string) => void
  onOpenOrSelect: (e: React.MouseEvent, taskId: string) => void
  onFileDragEnter: (taskId: string) => void
  onFileDragLeave: (taskId: string) => void
  onFileDrop: (task: Task, files: File[]) => void
}

// TaskCard is one board card, memoized.
//
// The board re-renders on every drag-over, file-drop hover and selection tick.
// Inline, that re-rendered EVERY card on the board for a state change that can
// only affect one of them. Memoized, a card re-renders only when its own task,
// derived meta, selection or drop-target flag changes — which is why the
// handlers arrive as stable identities (useStableCallback at the board level)
// and the volatile board state arrives already reduced to booleans.
function TaskCardImpl({
  task: t,
  meta,
  selected,
  fileDropActive,
  today,
  onDragStart,
  onOpenOrSelect,
  onFileDragEnter,
  onFileDragLeave,
  onFileDrop,
}: Props) {
  const { owner, flow, depIds, unmetDeps, unmetColColor } = meta
  // An optimistic card: created locally, still waiting for the server id/title.
  const pending = t.id.startsWith('temp-')

  return (
    <div
      data-testid="task-card"
      data-task-id={t.id}
      draggable={!pending}
      onDragStart={(e) => {
        if (pending) return
        onDragStart(t.id)
        e.dataTransfer.setData('application/x-tionswarm-task', t.id)
        e.dataTransfer.effectAllowed = 'link'
      }}
      onClick={(e) => {
        if (pending) return
        onOpenOrSelect(e, t.id)
      }}
      onDragOver={(e) => {
        // OS file drag over a card → offer to attach (a card being dragged
        // internally carries no 'Files', so moves are unaffected and still
        // bubble to the column).
        if (pending || !Array.from(e.dataTransfer.types).includes('Files')) return
        e.preventDefault()
        e.stopPropagation()
        onFileDragEnter(t.id)
      }}
      onDragLeave={(e) => {
        if (!Array.from(e.dataTransfer.types).includes('Files')) return
        onFileDragLeave(t.id)
      }}
      onDrop={(e) => {
        const files = Array.from(e.dataTransfer.files)
        if (files.length === 0) return // not a file drop → let the column handle the move
        e.preventDefault()
        e.stopPropagation()
        onFileDrop(t, files)
      }}
      className={`rounded-lg border bg-[var(--color-surface-2)] p-2 text-sm shadow-[var(--shadow-sm)] transition ${
        fileDropActive ? 'ring-2 ring-[var(--color-accent)] ring-offset-1' : ''
      } ${
        pending
          ? 'animate-pulse cursor-default border-[var(--color-border)] opacity-70'
          : `cursor-pointer hover:shadow-[var(--shadow-md)] active:cursor-grabbing ${
              selected
                ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] ring-1 ring-[var(--color-accent)]'
                : 'border-[var(--color-border)] hover:border-[var(--color-accent)]'
            }`
      }`}
    >
      <div className="font-medium">{t.title}</div>
      {pending ? (
        <div className="mt-1 text-[11px] text-[var(--color-text-dim)]">başlık üretiliyor…</div>
      ) : (
        t.description && (
          <div className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]">
            {t.description}
          </div>
        )
      )}
      {/* Rich attribute badges: due date, priority, tags. */}
      {(t.dueDate || t.priority || (t.tags?.length ?? 0) > 0) && (
        <div className="mt-1.5 flex flex-wrap items-center gap-1">
          {t.dueDate && (
            <span
              title={`Bitiş: ${t.dueDate}`}
              className={`inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium ${
                t.dueDate < today
                  ? 'bg-[var(--color-danger)]/15 text-[var(--color-danger)]'
                  : t.dueDate === today
                    ? 'bg-[var(--color-warning)]/15 text-[var(--color-warning)]'
                    : 'bg-[var(--color-surface)] text-[var(--color-text-dim)]'
              }`}
            >
              ◷ {t.dueDate.slice(5)}
            </span>
          )}
          {t.priority && PRIORITY_META[t.priority] && (
            <span
              className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium"
              style={{
                backgroundColor: PRIORITY_META[t.priority].color + '22',
                color: PRIORITY_META[t.priority].color,
              }}
            >
              ● {PRIORITY_META[t.priority].label}
            </span>
          )}
          {t.tags?.map((tag) => (
            <span
              key={tag}
              className="rounded-full bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]"
            >
              #{tag}
            </span>
          ))}
        </div>
      )}
      {(owner || t.flowId || depIds.length > 0 || (t.artifactIds?.length ?? 0) > 0) && (
        <div className="mt-2 flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
          {owner && <AgentIdentity agent={owner} size="sm" className="max-w-[160px]" />}
          {(t.artifactIds?.length ?? 0) > 0 && (
            <span
              className="inline-flex items-center gap-0.5 rounded bg-[var(--color-surface)] px-1.5 py-0.5 text-[10px]"
              title={`${t.artifactIds!.length} ek (artifact)`}
            >
              <Paperclip size={10} /> {t.artifactIds!.length}
            </span>
          )}
          {t.flowId && (
            <span className="inline-flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]">
              {normalizeAvatar(flow?.emoji) ?? '🔀'} {flow?.name ?? 'Akış'}
            </span>
          )}
          {depIds.length > 0 &&
            (() => {
              // unmetColColor (the first unmet dependency's column colour) comes
              // precomputed from the board's cardMeta.
              const chipStyle =
                unmetDeps.length > 0 && unmetColColor
                  ? { backgroundColor: unmetColColor + '22', color: unmetColColor }
                  : undefined
              return (
                <span
                  className={`inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] ${
                    unmetDeps.length > 0 && !unmetColColor
                      ? 'bg-[var(--color-warning)]/15 text-[var(--color-warning)]'
                      : unmetDeps.length > 0
                        ? ''
                        : 'bg-[var(--color-success)]/10 text-[var(--color-success)]'
                  }`}
                  style={chipStyle}
                  title={
                    unmetDeps.length > 0
                      ? `${unmetDeps.length} bağımlılık tamamlanmadı`
                      : 'Tüm bağımlılıklar tamamlandı'
                  }
                >
                  🔗 {depIds.length}
                </span>
              )
            })()}
        </div>
      )}
    </div>
  )
}

export const TaskCard = memo(TaskCardImpl)
