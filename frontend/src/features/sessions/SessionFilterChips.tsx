import type { MouseEvent as ReactMouseEvent } from 'react'
import { Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { modKeyLabel } from '@/shared/lib/platform'
import { SESSION_LIST_CHIPS, SUBAGENT_CHIP } from './sessionKindMeta'

interface Props {
  // Chips currently ticked (every chip starts ticked).
  chipSet: ReadonlySet<string>
  // chipKey → number of sessions that chip covers on the selected archive side.
  chipCounts: ReadonlyMap<string, number>
  onClickChip: (key: string, e: ReactMouseEvent) => void
}

// Chip tone. The accent is kept for a ticked chip that actually covers rows; a
// ticked chip with nothing behind it stays neutral so a strip of empty kinds
// does not read as eight highlighted filters.
function chipTone(on: boolean, empty: boolean): string {
  if (!on) {
    return 'text-[var(--color-text-dim)] opacity-60 hover:bg-[var(--color-surface-2)] hover:opacity-100'
  }
  if (empty) return 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
  return 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
}

// SessionFilterChips is the multi-select strip of session kinds plus the live
// scope. Unticking a chip hides that slice of the list.
export function SessionFilterChips({ chipSet, chipCounts, onClickChip }: Props) {
  const { t } = useTranslation('sessions')
  return (
    <div className="flex flex-wrap gap-1 px-3 pb-2" data-testid="session-kind-filters">
      {SESSION_LIST_CHIPS.map((f) => {
        const on = chipSet.has(f.key)
        const count = chipCounts.get(f.key) ?? 0
        const empty = count === 0
        const Icon = f.key === SUBAGENT_CHIP ? Sparkles : null
        return (
          <button
            key={f.key}
            onClick={(e) => onClickChip(f.key, e)}
            title={t('sidebar.filterTitle', { label: f.label, count, mod: modKeyLabel() })}
            aria-pressed={on}
            data-chip={f.key}
            data-empty={empty ? 'true' : undefined}
            className={`flex items-center gap-1 rounded-full px-2.5 py-1 text-[11px] transition ${chipTone(on, empty)}`}
          >
            {Icon && <Icon size={11} />}
            {f.label}
            <span className="opacity-60">{count}</span>
          </button>
        )
      })}
    </div>
  )
}
