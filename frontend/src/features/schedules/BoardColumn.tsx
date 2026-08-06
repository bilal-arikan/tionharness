import type { LucideIcon } from 'lucide-react'
import { Plus } from 'lucide-react'
import { LoadingState } from '@/shared/components'

interface Props {
  title: string
  icon: LucideIcon
  accent: string
  count: number
  /** Short explanation of what this column's rules do (column subtitle). */
  description: string
  /** Optional live metric badge shown under the description (e.g. current
   *  workspace token/message/tool totals for the scope this lane's rules watch). */
  stat?: React.ReactNode
  /** Opens the create popup for this column's kind. */
  onAdd: () => void
  addLabel: string
  loading: boolean
  loadingLabel: string
  emptyLabel: string
  testId?: string
  children: React.ReactNode
}

// BoardColumn is one lane of the automation board: a sticky colored header with
// the rule count and a "+" button that opens the create popup, over a scrollable
// card list. All three kinds (cron schedules, tag automations, board automations)
// use it so the lanes stay visually identical.
export function BoardColumn({
  title,
  icon: Icon,
  accent,
  count,
  description,
  stat,
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
      // Below `md` the three lanes scroll horizontally one-per-screen (snap), so a
      // portrait phone shows a full-width lane instead of three squeezed ones.
      className="flex min-h-0 w-[85vw] max-w-[340px] shrink-0 snap-center flex-col rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] md:w-auto md:max-w-none md:min-w-0 md:flex-1 md:snap-align-none"
    >
      <header
        className="flex shrink-0 items-start gap-2 rounded-t-lg border-b border-[var(--color-border)] border-t-2 px-3 py-2"
        style={{ borderTopColor: accent }}
      >
        <Icon size={16} className="mt-0.5 shrink-0" style={{ color: accent }} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-semibold text-[var(--color-text)]">{title}</span>
            <span className="rounded-full bg-[var(--color-surface)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]">
              {count}
            </span>
          </div>
          <p className="mt-0.5 text-[11px] leading-snug text-[var(--color-text-dim)]">
            {description}
          </p>
          {stat && (
            <div
              className="mt-1 inline-flex items-center gap-1 rounded bg-[var(--color-surface)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-text-dim)]"
              title="Bu workspace’in şu anki değeri — kuralların izlediği metrik (canlı)"
            >
              {stat}
            </div>
          )}
        </div>
        <button
          type="button"
          onClick={onAdd}
          title={addLabel}
          aria-label={addLabel}
          className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
        >
          <Plus size={14} />
        </button>
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
