import { memo } from 'react'
import { Paperclip, ArchiveRestore } from 'lucide-react'
import type { Agent, Flow, Task } from '@/types'
import { AgentIdentity } from '@/shared/components/agents/AgentIdentity'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { taskCardShadowClass } from './taskCardAppearance'

// Priority chip colors/labels, keyed by the stored priority slug.
const PRIORITY_META: Record<string, { label: string; color: string }> = {
  critical: { label: 'Kritik', color: 'var(--color-danger)' },
  high: { label: 'Yüksek', color: 'var(--color-warning)' },
  medium: { label: 'Orta', color: 'var(--color-accent)' },
  low: { label: 'Düşük', color: 'var(--color-text-dim)' },
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
  /** Preview of the last image attached to the card: the serving URL plus the
   *  artifact title for alt text. null when the card has no image attachment. */
  image: { url: string; title: string } | null
}

interface Props {
  task: Task
  meta: TaskCardMeta
  selected: boolean
  /** An OS file drag is hovering THIS card (ring highlight). */
  fileDropActive: boolean
  /** This card changed since the board was last opened — glows until the
   *  board view is left (the flag resets on unmount, not on a timer). */
  recentlyChanged: boolean
  /** This card's position among the board's current columns, for the keyboard
   *  move shortcut and its aria-label (e.g. "3 / 5"). */
  columnIndex: number
  columnCount: number
  columnLabel: string
  onDragStart: (taskId: string) => void
  onDragEnd: () => void
  onOpenOrSelect: (e: React.MouseEvent, taskId: string) => void
  onFileDragEnter: (taskId: string) => void
  onFileDragLeave: (taskId: string) => void
  onFileDrop: (task: Task, files: File[]) => void
  /** Keyboard equivalent of dragging the card to an adjacent column. */
  onMoveColumn: (taskId: string, direction: -1 | 1) => void
  /** When provided (archived view), renders a "restore" button on the card. Must
   *  be a stable identity so the card's memo still holds. */
  onUnarchive?: (task: Task) => void
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
  recentlyChanged,
  columnIndex,
  columnCount,
  columnLabel,
  onDragStart,
  onDragEnd,
  onOpenOrSelect,
  onFileDragEnter,
  onFileDragLeave,
  onFileDrop,
  onMoveColumn,
  onUnarchive,
}: Props) {
  const { owner, flow, depIds, unmetDeps, unmetColColor, image } = meta
  // An optimistic card: created locally, still waiting for the server id/title.
  const pending = t.id.startsWith('temp-')

  return (
    <div
      data-testid="task-card"
      data-task-id={t.id}
      draggable={!pending}
      tabIndex={pending ? -1 : 0}
      role="button"
      aria-label={`${t.title}${pending ? '' : `, görev kimliği ${t.id}`}, ${columnLabel} sütunu, ${columnIndex + 1}/${columnCount}. Taşımak için sol veya sağ ok tuşunu kullanın.`}
      onKeyDown={(e) => {
        if (pending) return
        // Ignore arrow keys while an input/textarea/select inside the card (if
        // any is ever added) or a nested control has focus, so the shortcut
        // never fights normal typing or page scroll.
        const targetTag = (e.target as HTMLElement).tagName
        if (targetTag === 'INPUT' || targetTag === 'TEXTAREA' || targetTag === 'SELECT') return
        if (e.key === 'ArrowLeft') {
          e.preventDefault()
          onMoveColumn(t.id, -1)
        } else if (e.key === 'ArrowRight') {
          e.preventDefault()
          onMoveColumn(t.id, 1)
        } else if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onOpenOrSelect(e as unknown as React.MouseEvent, t.id)
        }
      }}
      onDragStart={(e) => {
        if (pending) return
        onDragStart(t.id)
        e.dataTransfer.setData('application/x-tionharness-task', t.id)
        e.dataTransfer.effectAllowed = 'link'
      }}
      onDragEnd={onDragEnd}
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
      className={`relative rounded-lg border bg-[var(--color-surface-2)] p-2 text-sm transition focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:ring-offset-1 ${taskCardShadowClass(recentlyChanged, pending)} ${
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
      {!pending && (
        <span className="absolute right-2 top-2 font-mono text-[10px] leading-4 text-[var(--color-text-dim)] opacity-70">
          {t.id}
        </span>
      )}
      {onUnarchive && (
        <div className="mb-1">
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              onUnarchive(t)
            }}
            title="Arşivden çıkar (panoya geri al)"
            className="inline-flex items-center gap-1 rounded border border-[var(--color-border)] bg-[var(--color-surface)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
          >
            <ArchiveRestore size={11} /> Geri al
          </button>
        </div>
      )}
      {/* Attachment preview: the last image attached to the card, above the
          title. The box is a fixed 4:3 so the preview never grows taller than
          three quarters of the card width whatever the image's own ratio is;
          object-cover squeezes an off-ratio image into that box instead of
          letting it letterbox or push the rest of the card down. Cards without
          an image attachment render exactly as before — no placeholder. */}
      {image && (
        <div className="mb-2 aspect-[4/3] w-full overflow-hidden rounded-md border border-[var(--color-border)] bg-[var(--color-bg)]">
          <img
            src={image.url}
            alt={image.title}
            draggable={false}
            loading="lazy"
            className="h-full w-full object-cover"
          />
        </div>
      )}
      <div className={`${pending ? '' : 'pr-12'} font-medium`}>{t.title}</div>
      {pending ? (
        <div className="mt-1 text-[11px] text-[var(--color-text-dim)]">başlık üretiliyor…</div>
      ) : (
        t.description && (
          <div className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]">
            {t.description}
          </div>
        )
      )}
      {/* Rich attribute badges: priority and tags. */}
      {(t.priority || (t.tags?.length ?? 0) > 0) && (
        <div className="mt-1.5 flex flex-wrap items-center gap-1">
          {t.priority && PRIORITY_META[t.priority] && (
            <span
              className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium"
              style={{
                backgroundColor: `color-mix(in srgb, ${PRIORITY_META[t.priority].color} 14%, transparent)`,
                color: `color-mix(in srgb, ${PRIORITY_META[t.priority].color} 75%, var(--color-text))`,
              }}
            >
              ● {PRIORITY_META[t.priority].label}
            </span>
          )}
          {t.tags?.map((tag) => (
            <span
              key={tag}
              className="rounded-full bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[color-mix(in_srgb,var(--color-accent)_75%,var(--color-text))]"
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
            <span className="inline-flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[color-mix(in_srgb,var(--color-accent)_75%,var(--color-text))]">
              {normalizeAvatar(flow?.emoji) ?? '🔀'} {flow?.name ?? 'Akış'}
            </span>
          )}
          {depIds.length > 0 &&
            (() => {
              // unmetColColor (the first unmet dependency's column colour) comes
              // precomputed from the board's cardMeta.
              const chipStyle =
                unmetDeps.length > 0 && unmetColColor
                  ? {
                      backgroundColor: `color-mix(in srgb, ${unmetColColor} 14%, transparent)`,
                      color: `color-mix(in srgb, ${unmetColColor} 75%, var(--color-text))`,
                    }
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
