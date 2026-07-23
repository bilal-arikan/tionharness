import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { api } from '@/api'
import type { InsightRun } from '@/types'
import { relativeTime } from '@/shared/lib/time'

// RunsTab is the scan-run observability log (not sessions): when each scan ran,
// how long it took, and what it covered/produced.
export function RunsTab({ onError }: { onError: (msg: string) => void }) {
  const [runs, setRuns] = useState<InsightRun[]>([])
  const [loading, setLoading] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    api.getInsightRuns()
      .then(setRuns)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
  }, [onError])

  useEffect(load, [load])

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-sm text-[var(--color-text-dim)]">Son taramalar (session değil — kayıt log'u).</p>
        <button onClick={load} className="flex items-center gap-1 rounded-md px-2 py-1 text-sm hover:bg-[var(--color-surface-2)]">
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} /> Yenile
        </button>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead className="text-left text-xs text-[var(--color-text-dim)]">
            <tr className="border-b border-[var(--color-border)]">
              <th className="py-1 pr-3">Zaman</th>
              <th className="py-1 pr-3">Süre</th>
              <th className="py-1 pr-3">Oturum</th>
              <th className="py-1 pr-3">Analiz</th>
              <th className="py-1 pr-3">Atlandı</th>
              <th className="py-1 pr-3">Bulgu</th>
              <th className="py-1 pr-3">Hata</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((r, i) => (
              <tr key={i} className="border-b border-[var(--color-border)]">
                <td className="py-1 pr-3">{relativeTime(r.at)}</td>
                <td className="py-1 pr-3">{(r.durationMs / 1000).toFixed(1)}s</td>
                <td className="py-1 pr-3">{r.sessions}</td>
                <td className="py-1 pr-3">{r.analyzed}</td>
                <td className="py-1 pr-3">{r.skipped}</td>
                <td className="py-1 pr-3">{r.findings}</td>
                <td className={`py-1 pr-3 ${r.errors > 0 ? 'text-[var(--color-danger)]' : ''}`}>{r.errors}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {runs.length === 0 && !loading && (
          <div className="py-2 text-sm text-[var(--color-text-dim)]">Henüz tarama çalışmadı.</div>
        )}
      </div>
    </div>
  )
}
