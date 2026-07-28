import { useCallback, useEffect, useState } from 'react'
import { Loader2, Network, Users, CheckCircle2, Play, ChevronDown, ChevronRight, Workflow, ArrowLeft } from 'lucide-react'
import { api } from '@/api'
import { InfoPopover } from '@/shared/components/InfoPopover'
import { CoordinatorWorkflowPicker, WORKFLOW_HELP } from '@/shared/components/CoordinatorWorkflowPicker'
import { subscribeWorkerChange } from '@/shared/lib/workerBus'
import type { WorkerInfo } from '@/types'

interface Props {
  sessionId: string
  // The session's current role ('coordinator' | 'worker' | '' | undefined).
  role?: string
  // The session's selected coordinator recipe/workflow slug (M5), if any.
  workflow?: string
  // For a worker session: back-link to its coordinator, so the panel can offer a
  // "back to coordinator" shortcut.
  coordinatorSessionId?: string
  // Bumped by the parent whenever the conversation changes, so the worker list
  // refreshes as notifications land.
  refreshKey?: number
  onError: (msg: string) => void
  // Called after the role/workflow is toggled so the parent re-fetches session info.
  onRoleChanged: () => void
  // Navigates to a worker's own session when its roster row is clicked. Each worker
  // is a first-class session, so clicking opens its transcript.
  onSelectSession?: (id: string) => void
  // Opens the Skills screen on a given skill slug. A coordinator workflow IS a
  // skill (kind 'coordinator-workflow'), so this is how the picker links to the
  // recipe's actual instructions. Optional — the link hides when absent.
  onOpenSkill?: (slug: string) => void
}

// CoordinatorSection is the M2 coordination panel: it toggles a session into
// coordinator mode and, once on, shows the live worker roster (running vs
// finished, with each finished worker's one-line summary). See _Docs/47.
export function CoordinatorSection({ sessionId, role, workflow, coordinatorSessionId, refreshKey, onError, onRoleChanged, onSelectSession, onOpenSkill }: Props) {
  const isCoordinator = role === 'coordinator'
  const isWorker = role === 'worker'
  const [toggling, setToggling] = useState(false)
  const [savingWf, setSavingWf] = useState(false)
  const [workers, setWorkers] = useState<WorkerInfo[]>([])
  // Worker roster collapse (persisted) — the list can get long, so let it fold.
  const [workersOpen, setWorkersOpen] = useState(
    () => localStorage.getItem('tionswarm.coordWorkersOpen') !== '0',
  )
  // Which worker bucket is shown: running vs finished (persisted).
  const [workerTab, setWorkerTab] = useState<'running' | 'done'>(
    () => (localStorage.getItem('tionswarm.coordWorkerTab') === 'done' ? 'done' : 'running'),
  )
  const toggleWorkers = () =>
    setWorkersOpen((v) => {
      const next = !v
      localStorage.setItem('tionswarm.coordWorkersOpen', next ? '1' : '0')
      return next
    })
  const selectTab = (tab: 'running' | 'done') => {
    setWorkerTab(tab)
    localStorage.setItem('tionswarm.coordWorkerTab', tab)
  }

  const runningWorkers = workers.filter((w) => w.running)
  const doneWorkers = workers.filter((w) => !w.running)
  const shownWorkers = workerTab === 'running' ? runningWorkers : doneWorkers

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

  // Live worker transitions (start + completion) arrive over workerBus, fed by the
  // single SSE feed — so cards appear and flip to "finished" immediately, with no
  // polling and no dependency on the parent bumping refreshKey.
  useEffect(() => {
    if (!isCoordinator) return
    return subscribeWorkerChange(sessionId, loadWorkers)
  }, [isCoordinator, sessionId, loadWorkers])

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

  const selectWorkflow = async (next: string) => {
    setSavingWf(true)
    try {
      await api.setSessionWorkflow(sessionId, next)
      onRoleChanged()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSavingWf(false)
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
        {onSelectSession && coordinatorSessionId && (
          <button
            type="button"
            onClick={() => onSelectSession(coordinatorSessionId)}
            title="Koordinatör oturumunu aç"
            className="mt-1.5 flex w-full items-center justify-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-[11px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
          >
            <ArrowLeft size={13} className="shrink-0" /> Koordinatöre dön
          </button>
        )}
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

          {/* Workflow (recipe) picker (M5): a saved orchestration pattern layered on
              the coordinator prompt. */}
          <div className="rounded-lg border border-[var(--color-border)] px-2.5 py-2">
            {/* A plain heading, not a <label>: it labels no single control (the
                radiogroup below carries its own aria-label) and it now contains a
                button, which a label must not swallow clicks for. */}
            <div className="mb-1 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
              <Workflow size={12} /> Workflow
              {/* fixed: the session panel is `overflow-hidden` + `overflow-y-auto`
                  and barely wider than the bubble, so an absolutely-positioned one
                  is clipped on both axes no matter which edge it aligns to. */}
              <InfoPopover text={WORKFLOW_HELP} label="Workflow nedir?" fixed />
              {savingWf && <Loader2 size={11} className="animate-spin" />}
            </div>
            <CoordinatorWorkflowPicker
              value={workflow}
              onChange={selectWorkflow}
              disabled={savingWf}
              groupName={`coord-wf-${sessionId}`}
              onOpenSkill={onOpenSkill}
            />
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
            <>
              {/* Tabs: running vs finished, each with a live count. */}
              <div className="flex items-center gap-1 px-1">
                <button
                  onClick={() => selectTab('running')}
                  aria-pressed={workerTab === 'running'}
                  className={`flex items-center gap-1 rounded-md px-2 py-1 text-[10px] font-medium transition ${
                    workerTab === 'running'
                      ? 'bg-[var(--color-accent)]/10 text-[var(--color-accent)]'
                      : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                  }`}
                >
                  <Play size={11} className="shrink-0" /> Çalışan · {runningWorkers.length}
                </button>
                <button
                  onClick={() => selectTab('done')}
                  aria-pressed={workerTab === 'done'}
                  className={`flex items-center gap-1 rounded-md px-2 py-1 text-[10px] font-medium transition ${
                    workerTab === 'done'
                      ? 'bg-[var(--color-success)]/10 text-[var(--color-success)]'
                      : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                  }`}
                >
                  <CheckCircle2 size={11} className="shrink-0" /> Tamamlanan · {doneWorkers.length}
                </button>
              </div>
              {shownWorkers.length === 0 ? (
                <p className="px-1 text-[10px] text-[var(--color-text-dim)]">
                  {workerTab === 'running' ? 'Şu an çalışan worker yok.' : 'Henüz tamamlanan worker yok.'}
                </p>
              ) : (
                <ul className="space-y-1.5">
                  {shownWorkers.map((w) => {
                    // Each worker is its own session — clicking the row opens it.
                    const clickable = !!onSelectSession && !!w.sessionId
                    const inner = (
                      <>
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
                      </>
                    )
                    return (
                      <li key={w.sessionId}>
                        {clickable ? (
                          <button
                            type="button"
                            onClick={() => onSelectSession!(w.sessionId)}
                            title="Worker oturumunu aç"
                            className="block w-full rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-left transition hover:border-[var(--color-accent)] hover:bg-[var(--color-surface-2)]"
                          >
                            {inner}
                          </button>
                        ) : (
                          <div className="rounded-lg border border-[var(--color-border)] px-2.5 py-1.5">
                            {inner}
                          </div>
                        )}
                      </li>
                    )
                  })}
                </ul>
              )}
            </>
            )}
            </>
          )}
        </div>
      )}
    </section>
  )
}
