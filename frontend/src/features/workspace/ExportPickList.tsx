// Reusable multi-select checkbox list for the workspace export panel. Renders a
// titled section with a "select all / none" toggle and a grid of pickable rows,
// used identically for agents, flows, workspace skills and schedules so each
// category can be exported item-by-item (not just an all-or-nothing toggle).
import type { Dispatch, SetStateAction } from 'react'
import type { LucideIcon } from 'lucide-react'

// One selectable row. `emoji` (e.g. an agent avatar) takes precedence over the
// section icon; `sub` is an optional dimmed second line (cron, agent name, …).
export interface PickEntry {
  id: string
  label: string
  sub?: string
  emoji?: string
}

interface Props {
  title: string
  icon: LucideIcon
  entries: PickEntry[]
  picked: Set<string>
  setPicked: Dispatch<SetStateAction<Set<string>>>
  emptyHint: string
  // Optional note under the title (e.g. why some items are excluded by design).
  note?: string
}

export function ExportPickList({
  title,
  icon: Icon,
  entries,
  picked,
  setPicked,
  emptyHint,
  note,
}: Props) {
  const allPicked = entries.length > 0 && picked.size === entries.length
  const toggle = (id: string) =>
    setPicked((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  const toggleAll = () => setPicked(allPicked ? new Set() : new Set(entries.map((e) => e.id)))

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="flex items-center gap-2 text-sm font-semibold">
          <Icon size={16} className="text-[var(--color-accent)]" /> {title}
          <span className="text-xs font-normal text-[var(--color-text-dim)]">
            ({picked.size}/{entries.length})
          </span>
        </span>
        {entries.length > 0 && (
          <button
            onClick={toggleAll}
            className="text-xs text-[var(--color-accent)] hover:underline"
          >
            {allPicked ? 'Tümünü kaldır' : 'Tümünü seç'}
          </button>
        )}
      </div>
      {note && <p className="text-[11px] text-[var(--color-text-dim)]">{note}</p>}
      {entries.length === 0 ? (
        <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
          {emptyHint}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
          {entries.map((e) => {
            const on = picked.has(e.id)
            return (
              <button
                key={e.id}
                onClick={() => toggle(e.id)}
                className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-left text-sm transition ${
                  on
                    ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)]'
                    : 'border-[var(--color-border)] bg-[var(--color-bg)] hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <span
                  className={`flex h-4 w-4 flex-shrink-0 items-center justify-center rounded border text-[10px] ${
                    on
                      ? 'border-[var(--color-accent)] bg-[var(--color-accent)] text-white'
                      : 'border-[var(--color-border)]'
                  }`}
                >
                  {on ? '✓' : ''}
                </span>
                <span className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded bg-[var(--color-surface-2)] text-sm">
                  {e.emoji || <Icon size={14} />}
                </span>
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate">{e.label}</span>
                  {e.sub && (
                    <span className="truncate text-[11px] text-[var(--color-text-dim)]">
                      {e.sub}
                    </span>
                  )}
                </span>
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
