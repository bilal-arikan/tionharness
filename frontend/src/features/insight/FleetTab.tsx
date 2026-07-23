import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { api } from '@/api'
import type { FleetFinding } from '@/types'
import { SeverityBadge, RegressedBadge } from './insightBadges'

// FleetTab shows the fleet-wide app-fix backlog: the same TionSwarm bug surfacing
// across workspaces, deduped into one row with combined weight + origins.
export function FleetTab({ onError }: { onError: (msg: string) => void }) {
  const [rows, setRows] = useState<FleetFinding[]>([])
  const [loading, setLoading] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    api.getFleetFindings()
      .then(setRows)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
  }, [onError])

  useEffect(load, [load])

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-sm text-[var(--color-text-dim)]">
          Tüm workspace'lerin app-fix bulguları, kanonik imzayla birleştirilmiş.
        </p>
        <button onClick={load} className="flex items-center gap-1 rounded-md px-2 py-1 text-sm hover:bg-[var(--color-surface-2)]">
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} /> Yenile
        </button>
      </div>
      <div className="space-y-2">
        {rows.map((f) => (
          <div key={f.id} className="rounded-md border border-[var(--color-border)] p-3">
            <div className="flex flex-wrap items-center gap-2">
              {f.regressed && <RegressedBadge />}
              <span className="font-medium">{f.title}</span>
              {f.severity && <SeverityBadge severity={f.severity} />}
              <span className="text-xs text-[var(--color-text-dim)]">×{f.occurrences}</span>
            </div>
            {f.proposedFix && (
              <p className="mt-1 text-sm">
                <span className="font-medium">Öneri:</span> {f.proposedFix}
              </p>
            )}
            <div className="mt-2 flex flex-wrap items-center gap-1">
              <span className="text-xs text-[var(--color-text-dim)]">Workspace:</span>
              {f.workspaces.map((w) => (
                <span key={w} className="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-xs">
                  {w}
                </span>
              ))}
            </div>
          </div>
        ))}
        {rows.length === 0 && !loading && (
          <div className="text-sm text-[var(--color-text-dim)]">Fleet app-fix bulgusu yok.</div>
        )}
      </div>
    </div>
  )
}
