import type { LucideIcon } from 'lucide-react'
import { Plus } from 'lucide-react'
import { LoadingState } from '@/shared/components'

interface Props {
  title: string
  icon: LucideIcon
  /** Quiet tint of the lane icon; the lane frame itself is neutral. */
  accent: string
  count: number
  /** Short explanation of what this column's rules do (a line under the title). */
  description: string
  /** Opens the create popup for this column's kind. */
  onAdd: () => void
  addLabel: string
  loading: boolean
  loadingLabel: string
  emptyLabel: string
  testId?: string
  children: React.ReactNode
}

// BoardColumn is one lane of the automation board: a header with the title, the
// rule count and a "+" button that opens the create popup, over a scrollable
// card list. All three kinds (cron schedules, tag automations, board automations)
// use it so the lanes stay visually identical; only the icon's tint differs.
// The title owns its row (it wraps rather than truncating) and the description
// sits on its own line underneath, so a long description can never squeeze the
// title down to a few letters.
export function BoardColumn({
  title,
  icon: Icon,
  accent,
  count,
  description,
  onAdd,
  addLabel,
  loading,
  loadingLabel,
  emptyLabel,
  testId,
  children,
}: Props) {
  return (
    <section
      data-testid={testId}
      // Below `md` the lanes scroll horizontally one-per-screen (snap), so a
      // portrait phone shows a full-width lane instead of squeezed ones. On md+
      // a lane never shrinks below 17rem (the row scrolls sideways instead) and
      // grows up to 24rem when the board has room to spare.
      className="flex min-h-0 w-[85vw] max-w-[340px] shrink-0 snap-center flex-col rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] md:w-auto md:min-w-[17rem] md:max-w-[24rem] md:flex-1 md:snap-align-none 3xl:min-w-[19rem]"
    >
      <header className="shrink-0 rounded-t-lg border-b border-[var(--color-border)] px-3 py-2">
        <div className="flex items-center gap-2">
          <Icon size={16} className="shrink-0 opacity-80" style={{ color: accent }} />
          <h2 className="min-w-0 flex-1 break-words text-sm font-semibold text-[var(--color-text)]">
            {title}
          </h2>
          <span
            data-testid="board-column-count"
            className="shrink-0 rounded-full bg-[var(--color-surface)] px-1.5 py-0.5 text-[11px] text-[var(--color-text-dim)]"
          >
            {count}
          </span>
          <button
            type="button"
            onClick={onAdd}
            title={addLabel}
            aria-label={addLabel}
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] md:h-6 md:w-6"
          >
            <Plus size={14} />
          </button>
        </div>
        <p
          data-testid="board-column-description"
          className="mt-0.5 line-clamp-2 text-xs text-[var(--color-text-dim)]"
          title={description}
        >
          {description}
        </p>
      </header>
      <div className="min-h-0 flex-1 space-y-2 overflow-y-auto p-2">
        {loading && <LoadingState label={loadingLabel} />}
        {!loading && count === 0 && (
          <p className="px-1 py-4 text-center text-xs text-[var(--color-text-dim)]">{emptyLabel}</p>
        )}
        {children}
      </div>
    </section>
  )
}
