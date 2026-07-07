import { useCallback, useEffect, useState } from 'react'
import { Loader2, Network, Users, CheckCircle2, Play, ChevronDown, ChevronRight } from 'lucide-react'
import { api } from '@/api'
import type { WorkerInfo } from '@/types'

interface Props {
  sessionId: string
  // The session's current role ('coordinator' | 'worker' | '' | undefined).
  role?: string
  // Bumped by the parent whenever the conversation changes, so the worker list
  // refreshes as notifications land.
  refreshKey?: number
  onError: (msg: string) => void
  // Called after the role is toggled so the parent re-fetches session info.
  onRoleChanged: () => void
}

// CoordinatorSection is the M2 coordination panel: it toggles a session into
// coordinator mode and, once on, shows the live worker roster (running vs
// finished, with each finished worker's one-line summary). See _Docs/47.
export function CoordinatorSection({ sessionId, role, refreshKey, onError, onRoleChanged }: Props) {
  const isCoordinator = role === 'coordinator'
  const isWorker = role === 'worker'
  const [toggling, setToggling] = useState(false)
  const [workers, setWorkers] = useState<WorkerInfo[]>([])
  // Worker roster collapse (persisted) — the list can get long, so let it fold.
  const [workersOpen, setWorkersOpen] = useState(
    () => localStorage.getItem('tionswarm.coordWorkersOpen') !== '0',
  )
  const toggleWorkers = () =>
    setWorkersOpen((v) => {
      const next = !v
      localStorage.setItem('tionswarm.coordWorkersOpen', next ? '1' : '0')
      return next
    })

  const loadWorkers = useCallback(() => {
    if (!isCoordinator) return
    api
      .listWorkers(sessionId)
      .then((d) => setWorkers(d.workers ?? []))
      .catch(() => setWorkers([]))
  }, [isCoordinator, sessionId])

  // Refetch on mount, on session change, and whenever the parent bumps refreshKey
  // (a chat/worker event landed).
  useEffect(() => {
    loadWorkers()
  }, [loadWorkers, refreshKey])

  // While any worker is running, poll lightly so cards flip to "finished" without
  // depending on an event reaching the parent.
  useEffect(() => {
    if (!isCoordinator) return
    const anyRunning = workers.some((w) => w.running)
    if (!anyRunning) return
    const t = setInterval(loadWorkers, 3000)
    return () => clearInterval(t)
  }, [isCoordinator, workers, loadWorkers])

  const toggleRole = async (next: string) => {
    setToggling(true)
    try {
      await api.setSessionRole(sessionId, next)
      onRoleChanged()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setToggling(false)
    }
  }

  // A worker session shows only a passive note (its role is set at spawn time).
  if (isWorker) {
    return (
      <section>
        <div className="mb-2 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
          <Network size={12} /> <span>Koordinasyon</span>
        </div>
        <div className="rounded-lg border border-[var(--color-border)] px-2.5 py-2 text-[11px] text-[var(--color-text-dim)]">
          Bu oturum bir <span className="font-medium text-[var(--color-text)]">worker</span> — bir koordinatör tarafından başlatıldı. Sonucu, koordinatör oturumuna <code>&lt;task-notification&gt;</code> olarak iletilir.
        </div>
      </section>
    )
  }

  return (
    <section>
      <div className="mb-2 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
        <Network size={12} /> <span>Koordinasyon</span>
      </div>

      {!isCoordinator ? (
        <button
          onClick={() => toggleRole('coordinator')}
          disabled={toggling}
          className="flex w-full items-center gap-2 rounded-lg border border-dashed border-[var(--color-border)] px-2.5 py-2 text-left text-[11px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-50"
        >
          {toggling ? <Loader2 size={13} className="shrink-0 animate-spin" /> : <Users size={13} className="shrink-0" />}
          Koordinatör modunu aç (paralel worker'ları yönet)
        </button>
      ) : (
        <div className="space-y-2">
          <div className="flex items-center justify-between rounded-lg border border-[var(--color-accent)] bg-[var(--color-accent)]/5 px-2.5 py-2">
            <span className="flex items-center gap-1.5 text-[11px] font-medium text-[var(--color-accent)]">
              <Users size={13} /> Koordinatör modu açık
            </span>
            <button
              onClick={() => toggleRole('')}
              disabled={toggling}
              className="text-[10px] text-[var(--color-text-dim)] underline-offset-2 hover:underline disabled:opacity-50"
            >
              {toggling ? '…' : 'Kapat'}
            </button>
          </div>

          {workers.length === 0 ? (
            <p className="px-1 text-[11px] text-[var(--color-text-dim)]">
              Henüz worker yok. Sohbette <code>spawn_worker</code> ile paralel worker başlatın.
            </p>
          ) : (
            <>
            <button
              onClick={toggleWorkers}
              aria-expanded={workersOpen}
              className="flex w-full items-center gap-1.5 px-1 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              {workersOpen ? <ChevronDown size={12} className="shrink-0" /> : <ChevronRight size={12} className="shrink-0" />}
              Worker'lar · {workers.length}
            </button>
            {workersOpen && (
            <ul className="space-y-1.5">
              {workers.map((w) => (
                <li
                  key={w.sessionId}
                  className="rounded-lg border border-[var(--color-border)] px-2.5 py-1.5"
                >
                  <div className="flex items-center gap-1.5">
                    {w.running ? (
                      <Play size={12} className="shrink-0 text-[var(--color-accent)]" />
                    ) : (
                      <CheckCircle2 size={12} className="shrink-0 text-[var(--color-success)]" />
                    )}
                    <span className="truncate text-[11px] font-medium text-[var(--color-text)]">{w.agentName}</span>
                    <span className="ml-auto text-[9px] uppercase tracking-wide text-[var(--color-text-dim)]">
                      {w.running ? 'çalışıyor' : 'bitti'}
                    </span>
                  </div>
                  {w.summary && (
                    <p className="mt-0.5 line-clamp-2 text-[10px] text-[var(--color-text-dim)]">{w.summary}</p>
                  )}
                </li>
              ))}
            </ul>
            )}
            </>
          )}
        </div>
      )}
    </section>
  )
}
