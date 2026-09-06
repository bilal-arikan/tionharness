// Shared badge components for the Insight cockpit.

export function ChannelBadge({ channel }: { channel: string }) {
  const appFix = channel === 'app-fix'
  const recipe = channel === 'recipe-opt'
  const evolution = channel === 'evolution'
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-xs ${
        appFix
          ? 'bg-[var(--color-danger)]/15 text-[var(--color-danger)]'
          : recipe
            ? 'bg-[#6B7FD8]/15 text-[#6B7FD8]'
            : evolution
              ? 'bg-[#4F9E8F]/15 text-[#4F9E8F]'
              : 'bg-[var(--color-accent)]/15 text-[var(--color-accent)]'
      }`}
      title={
        recipe
          ? 'Reçete optimizer önerisi (Rota F4) — reçeteyi sen düzenlersin'
          : evolution
            ? 'Evrim önerisi (Hedefler) — kabul/ret senin; uygulama sonraki fazda'
            : undefined
      }
    >
      {appFix ? 'app-fix' : recipe ? '✦ recipe-opt' : evolution ? '🧬 evolution' : 'workspace-opt'}
    </span>
  )
}

export function SeverityBadge({ severity }: { severity: string }) {
  const color =
    severity === 'high'
      ? 'var(--color-danger)'
      : severity === 'med' || severity === 'medium'
        ? 'var(--color-warning)'
        : 'var(--color-text-dim)'
  return (
    <span
      className="rounded px-1.5 py-0.5 text-xs font-medium"
      style={{ color, backgroundColor: `color-mix(in srgb, ${color} 14%, transparent)` }}
    >
      {severity}
    </span>
  )
}

export function RegressedBadge() {
  return (
    <span className="rounded bg-[var(--color-danger)]/15 px-1.5 py-0.5 text-xs font-semibold text-[var(--color-danger)]">
      ⚠ REGRESYON
    </span>
  )
}

const STATUS_LABEL: Record<string, string> = {
  triaged: 'incelendi',
  accepted: 'kabul',
  applied: 'uygulandı',
  verified: 'doğrulandı',
  dismissed: 'yoksayıldı',
}

export function StatusBadge({ status }: { status: string }) {
  return (
    <span className="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-xs text-[var(--color-text-dim)]">
      {STATUS_LABEL[status] ?? status}
    </span>
  )
}
