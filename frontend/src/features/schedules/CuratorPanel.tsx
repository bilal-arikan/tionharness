// CuratorPanel — the Rota F3 curator's audit trail: what the weekly (or
// manual) LLM-free pass archived and what it only suggests, with a "run now"
// button. Nothing here deletes; archived entities are restorable.
import { useEffect, useState } from 'react'
import { Brush, Loader2, X } from 'lucide-react'
import { api } from '@/api'
import { Badge, ModalOverlay } from '@/shared/components'
import type { CuratorAction, CuratorReport } from '@/types/curator'
import { fmtTime } from './timeUtils'
import { CURATOR_ENTITY_LABEL, CURATOR_REASON_LABEL } from './curatorMeta'

interface Props {
  onClose: () => void
  onError: (msg: string) => void
  // Called after a manual run so the board reloads its lanes.
  onChanged: () => void
}

export function CuratorPanel({ onClose, onError, onChanged }: Props) {
  const [report, setReport] = useState<CuratorReport | null>(null)
  const [state, setState] = useState<'loading' | 'none' | 'ready'>('loading')
  const [running, setRunning] = useState(false)

  useEffect(() => {
    let cancelled = false
    api
      .curatorReport()
      .then((r) => {
        if (!cancelled) {
          setReport(r)
          setState('ready')
        }
      })
      .catch(() => {
        if (!cancelled) setState('none')
      })
    return () => {
      cancelled = true
    }
  }, [])

  const run = async (apply: boolean) => {
    setRunning(true)
    try {
      const r = await api.runCurator(apply)
      setReport(r)
      setState('ready')
      if (apply) onChanged()
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    } finally {
      setRunning(false)
    }
  }

  return (
    <ModalOverlay onClose={onClose}>
      <div
        className="flex max-h-[80vh] w-[min(720px,92vw)] flex-col rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
        data-testid="curator-panel"
      >
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-4 py-3">
          <Brush size={16} className="opacity-70" />
          <span className="text-sm font-semibold">Küratör</span>
          <span className="text-xs text-[var(--color-text-dim)]">
            haftalık, boşta tetiklenir · yalnız arşivler, asla silmez · sabitlenenler muaf
          </span>
          <button
            type="button"
            onClick={onClose}
            className="ml-auto rounded p-1 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            aria-label="Kapat"
          >
            <X size={16} />
          </button>
        </div>
        <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-4 py-2 text-xs">
          {state === 'loading' ? (
            <span className="text-[var(--color-text-dim)]">Yükleniyor…</span>
          ) : report ? (
            <>
              <span>
                Son geçiş: {fmtTime(report.at)} ·{' '}
                {report.trigger === 'weekly' ? 'haftalık' : 'elle'}
                {report.idle ? '' : ' · workspace meşguldü'}
              </span>
              <Badge tone={report.archived > 0 ? 'warning' : 'muted'}>
                {report.archived} arşivlendi
              </Badge>
              <Badge tone={report.suggestions > 0 ? 'accent' : 'muted'}>
                {report.suggestions} öneri
              </Badge>
            </>
          ) : (
            <span className="text-[var(--color-text-dim)]">
              Küratör bu workspace'te henüz çalışmadı. İlk geçiş, bir hafta boşta kalınca ya da
              aşağıdan elle.
            </span>
          )}
          <span className="ml-auto flex items-center gap-1.5">
            <button
              type="button"
              disabled={running}
              onClick={() => run(false)}
              className="rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-[var(--color-text-dim)] hover:text-[var(--color-accent)] disabled:opacity-40"
              title="Yalnız öneri üret; hiçbir şeyi arşivleme"
              data-testid="curator-dry-run"
            >
              Kuru çalıştır
            </button>
            <button
              type="button"
              disabled={running}
              onClick={() => run(true)}
              className="flex items-center gap-1 rounded-lg border border-[var(--color-accent)] px-2.5 py-1 text-[var(--color-accent)] disabled:opacity-40"
              title="Ajan yapımı tükenmiş / süresi dolmuş kuralları arşivle, kalanını öner"
              data-testid="curator-run"
            >
              {running ? <Loader2 size={12} className="animate-spin" /> : <Brush size={12} />}
              Şimdi çalıştır
            </button>
          </span>
        </div>
        <div className="min-h-0 flex-1 overflow-auto px-4 py-3 text-xs">
          {report && report.actions.length === 0 && (
            <p className="text-[var(--color-text-dim)]">
              Temiz: ne arşivlenecek ne önerilecek bir şey var.
            </p>
          )}
          {report && report.actions.length > 0 && (
            <ul className="flex flex-col gap-1.5">
              {report.actions.map((a, i) => (
                <CuratorRow key={`${a.entity}:${a.id}:${a.reason}:${i}`} action={a} />
              ))}
            </ul>
          )}
        </div>
      </div>
    </ModalOverlay>
  )
}

function CuratorRow({ action: a }: { action: CuratorAction }) {
  return (
    <li className="flex flex-wrap items-center gap-2 rounded border border-[var(--color-border)] px-2.5 py-1.5">
      <Badge tone={a.applied ? 'warning' : 'accent'}>{a.applied ? 'arşivlendi' : 'öneri'}</Badge>
      <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]">
        {CURATOR_ENTITY_LABEL[a.entity] ?? a.entity}
      </span>
      <span className="font-medium">{a.name || a.id}</span>
      <span className="text-[var(--color-text-dim)]">
        {CURATOR_REASON_LABEL[a.reason] ?? a.reason}
      </span>
      {a.detail && <span className="text-[var(--color-text-dim)]">· {a.detail}</span>}
      {a.evidence && (
        <span className="ml-auto text-[10px] text-[var(--color-text-dim)]" title="Kanıt">
          {a.evidence}
        </span>
      )}
    </li>
  )
}
