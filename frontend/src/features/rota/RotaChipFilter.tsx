// Rota's lane filter strip: the same multi-select chips the session sidebar
// puts above its list, so a chip reads the same on both screens. Plain click
// toggles one chip, Ctrl/Cmd-click solos it, Shift-click inverts the rest.
import type { MouseEvent as ReactMouseEvent } from 'react'
import { Archive, Sparkles, Users } from 'lucide-react'
import {
  ARCHIVED_CHIP,
  SESSION_CHIPS,
  SUBAGENT_CHIP,
  WORKER_CHIP,
  type ChipClickMode,
} from '@/features/sessions/sessionKindMeta'

interface Props {
  chipSet: ReadonlySet<string>
  counts: Map<string, number>
  onClickChip: (key: string, mode: ChipClickMode) => void
  onReset: () => void
  // How many chips are unticked; the reset button only shows when it is > 0.
  offCount: number
}

export function RotaChipFilter({ chipSet, counts, onClickChip, onReset, offCount }: Props) {
  const click = (key: string, e: ReactMouseEvent) => {
    const mode: ChipClickMode = e.ctrlKey || e.metaKey ? 'solo' : e.shiftKey ? 'invert' : 'toggle'
    onClickChip(key, mode)
  }
  return (
    <div
      className="flex flex-wrap items-center gap-1 border-b border-[var(--color-border)] px-4 py-1.5"
      data-testid="rota-chip-filter"
    >
      {SESSION_CHIPS.map((f) => {
        const on = chipSet.has(f.key)
        const Icon =
          f.key === WORKER_CHIP
            ? Users
            : f.key === ARCHIVED_CHIP
              ? Archive
              : f.key === SUBAGENT_CHIP
                ? Sparkles
                : null
        return (
          <button
            key={f.key}
            type="button"
            onClick={(e) => click(f.key, e)}
            title={`${f.label}: ${counts.get(f.key) ?? 0} şerit — Ctrl: yalnız bunu seç, Shift: diğerlerini tersle`}
            aria-pressed={on}
            data-chip={f.key}
            className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] transition ${
              on
                ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                : 'text-[var(--color-text-dim)] opacity-60 hover:bg-[var(--color-surface-2)] hover:opacity-100'
            }`}
          >
            {Icon && <Icon size={11} />}
            {f.label}
            <span className="opacity-60">{counts.get(f.key) ?? 0}</span>
          </button>
        )
      })}
      {offCount > 0 && (
        <button
          type="button"
          onClick={onReset}
          className="ml-1 rounded px-1.5 py-0.5 text-[11px] text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
          title={`${offCount} süzgeç kapalı — hepsini geri aç`}
        >
          süzgeci sıfırla
        </button>
      )}
    </div>
  )
}
