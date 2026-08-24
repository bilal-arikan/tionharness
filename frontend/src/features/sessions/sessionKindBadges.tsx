// Session run-state badges. Split out of sessionKindMeta so that module stays
// component-free (fast refresh only preserves state for component-only files).
import { Badge } from '@/shared/components'
import { runStateMeta } from './runStateMeta'

// RunStateBadge renders a session's last run outcome, or nothing when the session
// has never run a background turn (or carries a value this UI does not know). The
// label/tone table and the never-ran rule live in ./runStateMeta.
export function RunStateBadge({ runState }: { runState?: string }) {
  const meta = runStateMeta(runState)
  if (!meta) return null
  return (
    <Badge tone={meta.tone} className="shrink-0">
      <span title={meta.title}>{meta.label}</span>
    </Badge>
  )
}

// StatusPill shows a finished run's pass/fail outcome (task/flow kinds).
export function StatusPill({ status }: { status: string }) {
  if (status !== 'success' && status !== 'failure') return null
  const ok = status === 'success'
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-semibold"
      style={{
        color: ok ? 'var(--color-success)' : 'var(--color-danger)',
        backgroundColor: ok
          ? 'color-mix(in srgb, var(--color-success) 14%, transparent)'
          : 'color-mix(in srgb, var(--color-danger) 14%, transparent)',
      }}
    >
      {ok ? 'başarılı' : 'hata'}
    </span>
  )
}
