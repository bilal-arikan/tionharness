import type { DashboardCounters } from '@/types'

interface Tile {
  label: string
  value: number
  // hint is the secondary line: context that turns a bare number into a state.
  hint?: string
  // tone colours the value when the number itself is the warning.
  tone?: 'normal' | 'warn' | 'danger'
}

// StatTiles is the headline row. Only counts that a reader can ACT on are here —
// a tile that never changes teaches people to stop looking at the row.
export function StatTiles({ c }: { c: DashboardCounters }) {
  const tiles: Tile[] = [
    { label: 'Ajan', value: c.agents },
    {
      label: 'Aktif oturum',
      value: c.sessionsActive,
      hint: `${c.sessions} toplam${c.sessionsArchived > 0 ? ` · ${c.sessionsArchived} arşiv` : ''}`,
    },
    {
      label: 'Takılmış oturum',
      value: c.sessionsStuck,
      // Zero is the healthy value here, so it must not shout.
      tone: c.sessionsStuck > 0 ? 'danger' : 'normal',
      hint: c.sessionsStuck > 0 ? 'StuckTurns > 0' : 'temiz',
    },
    { label: 'Açık kart', value: c.tasksOpen, hint: `${c.tasks} toplam` },
    {
      label: 'Çalışan koşu',
      value: c.runsRunning,
      tone: c.runsRunning > 0 ? 'warn' : 'normal',
      hint: c.runsWaiting > 0 ? `${c.runsWaiting} girdi bekliyor` : `${c.runs} toplam`,
    },
    {
      label: 'Başarısız koşu',
      value: c.runsFailed,
      tone: c.runsFailed > 0 ? 'danger' : 'normal',
      hint: c.runsFailed > 0 ? 'incelenmeli' : 'temiz',
    },
  ]

  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6 square:grid-cols-3">
      {tiles.map((t) => (
        <div
          key={t.label}
          className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
        >
          <div className="text-xs text-[var(--color-text-dim)]">{t.label}</div>
          <div
            className={`mt-0.5 text-2xl font-semibold tabular-nums ${
              t.tone === 'danger'
                ? 'text-[var(--color-danger)]'
                : t.tone === 'warn'
                  ? 'text-[var(--color-accent)]'
                  : ''
            }`}
          >
            {t.value}
          </div>
          {t.hint && (
            <div className="mt-0.5 truncate text-[11px] text-[var(--color-text-dim)]">{t.hint}</div>
          )}
        </div>
      ))}
    </div>
  )
}
