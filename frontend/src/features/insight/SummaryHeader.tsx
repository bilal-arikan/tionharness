import type { FindingSummary } from './insightHelpers'

interface Props {
  summary: FindingSummary
  onPick: (patch: { channel?: string; severity?: string; regressedOnly?: boolean }) => void
}

// SummaryHeader is the triage cockpit's at-a-glance strip: clickable stat chips
// that double as one-tap filters.
export function SummaryHeader({ summary, onPick }: Props) {
  const chips: { label: string; value: number; danger?: boolean; onClick: () => void }[] = [
    { label: 'Toplam', value: summary.total, onClick: () => onPick({}) },
    { label: 'Açık', value: summary.open, onClick: () => onPick({}) },
    { label: 'app-fix', value: summary.appFix, onClick: () => onPick({ channel: 'app-fix' }) },
    {
      label: 'workspace-opt',
      value: summary.workspaceOpt,
      onClick: () => onPick({ channel: 'workspace-opt' }),
    },
    {
      label: '✦ recipe-opt',
      value: summary.recipeOpt,
      onClick: () => onPick({ channel: 'recipe-opt' }),
    },
    {
      label: '🧬 evolution',
      value: summary.evolution,
      onClick: () => onPick({ channel: 'evolution' }),
    },
    { label: 'yüksek', value: summary.high, onClick: () => onPick({ severity: 'high' }) },
    {
      label: '⚠ regresyon',
      value: summary.regressed,
      danger: true,
      onClick: () => onPick({ regressedOnly: true }),
    },
  ]
  return (
    <div className="flex flex-wrap gap-2">
      {chips.map((c) => (
        <button
          key={c.label}
          onClick={c.onClick}
          className={`flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-sm hover:bg-[var(--color-surface-2)] ${
            c.danger && c.value > 0
              ? 'border-[var(--color-danger)]/40 text-[var(--color-danger)]'
              : 'border-[var(--color-border)]'
          }`}
        >
          <span className="font-semibold">{c.value}</span>
          <span className="text-[var(--color-text-dim)]">{c.label}</span>
        </button>
      ))}
    </div>
  )
}
