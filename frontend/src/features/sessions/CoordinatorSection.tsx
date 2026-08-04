import { useCallback, useEffect, useState } from 'react'
import {
  Loader2,
  Network,
  Users,
  CheckCircle2,
  Play,
  ChevronDown,
  ChevronRight,
  Workflow,
  ArrowLeft,
  OctagonAlert,
  RotateCw,
  Inbox,
} from 'lucide-react'
import { api } from '@/api'
import { InfoPopover } from '@/shared/components/InfoPopover'
import {
  CoordinatorWorkflowPicker,
  WORKFLOW_HELP,
} from '@/shared/components/CoordinatorWorkflowPicker'
import { subscribeWorkerChange } from '@/shared/lib/workerBus'
import { isCoordinatorSession, isWorkerSession } from '@/shared/lib/coordination'
import { CoordinatorBreadcrumb } from './CoordinatorBreadcrumb'
import { CoordinatorTreeView } from './CoordinatorTreeView'
import type { WorkerInfo } from '@/types'

interface Props {
  sessionId: string
  // Lineage: 'worker' when a coordinator spawned this session, else '' — NOT the
  // coordinator capability, which is `coordinatorMode`. The two are independent
  // since coordinator trees can nest (a mid-level node has both).
  role?: string
  // Whether this session may drive workers of its own.
  coordinatorMode?: boolean
  // This session's depth in its coordinator tree (root = 0).
  coordinatorDepth?: number
  // True when the phantom-spawn stall guard hard-halted this coordinator's auto-turns.
  // Drives the persistent "durduruldu" badge + resume CTA below.
  stallHalted?: boolean
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
export function CoordinatorSection({
  sessionId,
  role,
  coordinatorMode,
  coordinatorDepth,
  workflow,
  coordinatorSessionId,
  stallHalted,
  refreshKey,
  onError,
  onRoleChanged,
  onSelectSession,
  onOpenSkill,
}: Props) {
  const session = { role, coordinatorMode, coordinatorSessionId, coordinatorDepth }
  const isCoordinator = isCoordinatorSession(session)
  const isWorker = isWorkerSession(session)
  const [toggling, setToggling] = useState(false)
  const [savingWf, setSavingWf] = useState(false)
  const [resuming, setResuming] = useState(false)
  const [workers, setWorkers] = useState<WorkerInfo[]>([])
  // Worker roster collapse (persisted) — the list can get long, so let it fold.
  const [workersOpen, setWorkersOpen] = useState(
    () => localStorage.getItem('tionswarm.coordWorkersOpen') !== '0',
  )
  // Which worker bucket is shown: running vs finished (persisted).
  const [workerTab, setWorkerTab] = useState<'running' | 'done'>(() =>
    localStorage.getItem('tionswarm.coordWorkerTab') === 'done' ? 'done' : 'running',
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

  const resumeCoordinator = async () => {
    setResuming(true)
    try {
      await api.resumeCoordinator(sessionId)
      // Refetch session info so the halt badge clears once the state flips.
      onRoleChanged()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setResuming(false)
    }
  }

  // A worker's upward context. Rendered as a HEADER, not an early return: a
  // mid-level node is a worker AND a coordinator, so it needs both this block and
  // the coordinator controls below it.
  const workerHeader = isWorker && (
    <div className="mb-2 space-y-1.5">
      <div className="rounded-lg border border-[var(--color-border)] px-2.5 py-2 text-[11px] text-[var(--color-text-dim)]">
        {isCoordinator ? (
          <>
            Bu oturum bir{' '}
            <span className="font-medium text-[var(--color-text)]">alt-koordinatör</span> — hem
            kendi worker'larını yönetir hem de üstündeki koordinatöre rapor verir. Turu bitmesi
            işinin bittiği anlamına gelmez; sonucu <code>report_to_coordinator</code> ile kapatır.
          </>
        ) : (
          <>
            Bu oturum bir <span className="font-medium text-[var(--color-text)]">worker</span> — bir
            koordinatör tarafından başlatıldı. Sonucu, koordinatör oturumuna{' '}
            <code>&lt;task-notification&gt;</code> olarak iletilir.
          </>
        )}
      </div>
      <CoordinatorBreadcrumb sessionId={sessionId} onSelectSession={onSelectSession} />
      {onSelectSession && coordinatorSessionId && (
        <button
          type="button"
          onClick={() => onSelectSession(coordinatorSessionId)}
          title="Koordinatör oturumunu aç"
          className="flex w-full items-center justify-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-[11px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
        >
          <ArrowLeft size={13} className="shrink-0" /> Koordinatöre dön
        </button>
      )}
    </div>
  )

  return (
    <section>
      <div className="mb-2 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
        <Network size={12} /> <span>Koordinasyon</span>
      </div>

      {workerHeader}

      {/* The full tree, for anyone inside one. The roster below shows only DIRECT
          workers — complete while trees were one level deep, but blind to
          everything a sub-coordinator is running (and to most of the cost). */}
      {(isCoordinator || isWorker) && (
        <div className="mb-2">
          <CoordinatorTreeView
            sessionId={sessionId}
            refreshKey={refreshKey}
            onSelectSession={onSelectSession}
          />
        </div>
      )}

      {!isCoordinator ? (
        <button
          onClick={() => toggleRole('coordinator')}
          disabled={toggling}
          className="flex w-full items-center gap-2 rounded-lg border border-dashed border-[var(--color-border)] px-2.5 py-2 text-left text-[11px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-50"
        >
          {toggling ? (
            <Loader2 size={13} className="shrink-0 animate-spin" />
          ) : (
            <Users size={13} className="shrink-0" />
          )}
          {isWorker
            ? "Koordinatör modunu aç (bu worker kendi worker'larını yönetsin)"
            : "Koordinatör modunu aç (paralel worker'ları yönet)"}
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

          {/* Phantom-spawn hard-halt: a persistent, actionable banner (not a transient
              toast). Shown until the coordinator recovers (a real spawn_worker call) or
              the user resumes it here. */}
          {stallHalted && (
            <div className="space-y-1.5 rounded-lg border border-[var(--color-error)] bg-[var(--color-error)]/5 px-2.5 py-2">
              <div className="flex items-center gap-1.5 text-[11px] font-semibold text-[var(--color-error)]">
                <OctagonAlert size={13} className="shrink-0" /> Koordinatör durduruldu
              </div>
              <p className="text-[10px] leading-relaxed text-[var(--color-text-dim)]">
                Koordinatör worker başlattığını anlatıp gerçek bir <code>spawn_worker</code> çağrısı
                yapmadı; düzeltici uyarılar sonuç vermedi, otomatik turlar durduruldu. Sohbete{' '}
                <code>spawn_worker'ı gerçekten çağır</code> gibi bir mesaj yazın ya da aşağıdan tek
                turluk devam ettirin.
              </p>
              <button
                onClick={resumeCoordinator}
                disabled={resuming}
                className="flex items-center gap-1.5 rounded-md border border-[var(--color-error)] px-2 py-1 text-[10px] font-medium text-[var(--color-error)] transition hover:bg-[var(--color-error)]/10 disabled:opacity-50"
              >
                {resuming ? (
                  <Loader2 size={12} className="shrink-0 animate-spin" />
                ) : (
                  <RotateCw size={12} className="shrink-0" />
                )}
                Devam ettir
              </button>
            </div>
          )}

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
                {workersOpen ? (
                  <ChevronDown size={12} className="shrink-0" />
                ) : (
                  <ChevronRight size={12} className="shrink-0" />
                )}
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
                      <CheckCircle2 size={11} className="shrink-0" /> Tamamlanan ·{' '}
                      {doneWorkers.length}
                    </button>
                  </div>
                  {shownWorkers.length === 0 ? (
                    <p className="px-1 text-[10px] text-[var(--color-text-dim)]">
                      {workerTab === 'running'
                        ? 'Şu an çalışan worker yok.'
                        : 'Henüz tamamlanan worker yok.'}
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
                                <CheckCircle2
                                  size={12}
                                  className="shrink-0 text-[var(--color-success)]"
                                />
                              )}
                              <span className="truncate text-[11px] font-medium text-[var(--color-text)]">
                                {w.agentName}
                              </span>
                              {/* "delegating" is NOT "running": a sub-coordinator between its
                              own turns has no live turn, it is waiting on its branch.
                              Labelling it "çalışıyor" would suggest an answer is coming;
                              labelling it "bitti" would be worse still — its result does
                              not exist yet. */}
                              {/* A parked follow-up (send_to_worker while the worker was
                              busy): it will be delivered the instant this turn ends. The
                              badge tells the coordinator the message landed, so it need
                              not resend or reach for stop_worker. */}
                              {w.running && w.queued && (
                                <span
                                  title="Bekleyen mesaj: bu tur bitince otomatik teslim edilecek"
                                  className="ml-auto flex items-center gap-1 rounded-full bg-[var(--color-warning)]/10 px-1.5 py-0.5 text-[9px] font-medium uppercase tracking-wide text-[var(--color-warning)]"
                                >
                                  <Inbox size={9} className="shrink-0" /> kuyrukta
                                </span>
                              )}
                              <span
                                className={`text-[9px] uppercase tracking-wide text-[var(--color-text-dim)] ${
                                  w.running && w.queued ? '' : 'ml-auto'
                                }`}
                              >
                                {w.delegating ? 'dağıtıyor' : w.running ? 'çalışıyor' : 'bitti'}
                              </span>
                            </div>
                            {w.summary && (
                              <p className="mt-0.5 line-clamp-2 text-[10px] text-[var(--color-text-dim)]">
                                {w.summary}
                              </p>
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
