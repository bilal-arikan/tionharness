import { useEffect, useMemo, useState } from 'react'
import {
  Trash2,
  Archive,
  ArchiveRestore,
  Bug,
  ChevronDown,
  ChevronRight,
  Pin,
  PinOff,
} from 'lucide-react'
import { api } from '@/api'
import type { SessionInfo, SessionUsageDetail } from '@/types'
import { ViewPanel } from '@/features/view/ViewPanel'
import { Badge, KeyValueRow as Row, TagEditor } from '@/shared/components'
import { ResizeHandle } from '@/shared/components/SidebarChrome'
import { useResizableSidebar } from '@/shared/hooks/useResizableSidebar'
import { Section, ActionBtn } from './SessionDetailBits'
import { SessionTitleBlock } from './SessionTitleBlock'
import { SessionProcessCard } from './SessionProcessCard'
import { SessionContextUsage } from './SessionContextUsage'
import { SessionAgentsSection } from './SessionAgentsSection'
import { SessionUsageCard } from './SessionUsageCard'
import { SessionExecutionCard } from './SessionExecutionCard'
import { CacheWarmthBadge } from './CacheWarmthBadge'
import { formatBytes, formatDate, cacheRemaining } from './sessionDetailFormat'
import { serverNow } from '@/shared/lib/serverClock'
import { isWritableSessionKind } from '@/shared/lib/sessionKind'
import { subscribeWorkerChange } from '@/shared/lib/workerBus'

interface Props {
  sessionId: string
  // Bumped by the parent whenever the conversation changes, so size/context
  // figures refresh without reselecting the session.
  refreshKey?: number
  onClose: () => void
  onError: (msg: string) => void
  onGenerateTitle: (id: string) => void | Promise<void>
  onRename: (id: string, title: string) => void | Promise<void>
  onDeleteSession: (id: string) => void
  // Pin/unpin the session so the sidebar list reorders immediately. Optional so
  // legacy/test usages still compile; the panel falls back to the raw API call.
  onSetPinned?: (id: string, pinned: boolean) => void
  // Navigate to the Agents view and focus the given agent. Wired so a click on
  // a participant chip in the "Konuşmadaki ajanlar" section jumps to that
  // agent's page (matches the behaviour of clicking the agent in the Agents
  // roster). Optional so legacy/test usages still compile.
  onSelectAgent?: (id: string) => void
  // Navigate to another session (used by the context-reset lineage link).
  onSelectSession?: (id: string) => void
  // Restart the last turn (stop any in-flight run + re-send the last user prompt).
  // Wired to the chat hook's rerunLast so it reuses the one true turn path. Optional
  // so legacy/test usages still compile; the button hides when absent.
  onRerun?: (id: string) => void | Promise<void>
  // Open the Debug / observability panel (SessionDebugModal). Lives here in the
  // action list rather than in the chat header. Optional so legacy/test usages
  // still compile; the button hides when absent.
  onOpenDebug?: () => void
}

// SessionDetailPanel is the right-hand inspector for the active chat session:
// on-disk footprint, context composition, participating agents and quick actions.
export function SessionDetailPanel({
  sessionId,
  refreshKey,
  onClose,
  onError,
  onGenerateTitle,
  onRename,
  onDeleteSession,
  onSetPinned,
  onSelectSession,
  onSelectAgent,
  onRerun,
  onOpenDebug,
}: Props) {
  // Persisted, drag-resizable width. The panel sits on the RIGHT, so its handle
  // is on the LEFT edge and the drag direction is inverted (drag left = wider).
  const { width, startDrag } = useResizableSidebar({
    storageKey: 'tionharness.sessionInfoWidth',
    defaultWidth: 320,
    min: 280,
    max: 640,
    invert: true,
    // Docked only on wide/ultra (a drawer elsewhere), so no tier cap.
    capToTier: false,
  })
  const [info, setInfo] = useState<SessionInfo | null>(null)
  // In-flight action guard for the running-process card (stop/restart/drop).
  const [procBusy, setProcBusy] = useState<'' | 'stop' | 'restart' | 'drop'>('')
  // Ticks once a second while a turn is running, so the elapsed timer is live.
  const [nowTick, setNowTick] = useState(() => serverNow())
  const [sessionUsage, setSessionUsage] = useState<SessionUsageDetail | null>(null)
  const [loading, setLoading] = useState(false)
  const [titling, setTitling] = useState(false)
  // In-flight guard for the archive / unarchive toggle.
  const [archiving, setArchiving] = useState(false)
  // In-flight guard for the pin / unpin toggle.
  const [pinning, setPinning] = useState(false)
  // Manual rename: when editing, hold the draft text; saving persists verbatim.
  const [editingTitle, setEditingTitle] = useState(false)
  const [titleDraft, setTitleDraft] = useState('')
  const [savingTitle, setSavingTitle] = useState(false)
  // Manual-refresh nonce: bumped by the refresh button (and after a title
  // regeneration) to re-fetch without touching the parent's refreshKey.
  const [localRefresh, setLocalRefresh] = useState(0)
  // "Özet" (session projection) collapse — persisted, defaults open. The DSL block
  // can get tall, so let it fold away like the coordinator section used to.
  const [summaryOpen, setSummaryOpen] = useState(
    () => localStorage.getItem('tionharness.sessionSummaryOpen') !== '0',
  )
  const toggleSummary = () =>
    setSummaryOpen((v) => {
      const next = !v
      localStorage.setItem('tionharness.sessionSummaryOpen', next ? '1' : '0')
      return next
    })

  useEffect(() => {
    let alive = true
    setLoading(true)
    api
      .sessionInfo(sessionId)
      .then((d) => alive && setInfo(d))
      .catch((e) => alive && onError((e as Error).message))
      .finally(() => alive && setLoading(false))
    return () => {
      alive = false
    }
  }, [sessionId, refreshKey, localRefresh, onError])

  // This session's own lifetime spend + savings. Refetched on the same triggers
  // so a finished turn updates the figure.
  useEffect(() => {
    let alive = true
    api
      .sessionUsageDetail(sessionId)
      .then((u) => alive && setSessionUsage(u))
      .catch(() => alive && setSessionUsage(null))
    return () => {
      alive = false
    }
  }, [sessionId, refreshKey, localRefresh])

  // Live coordination refresh: a worker transition OR a coordination signal for this
  // session (fed onto workerBus by useAppEvents) refetches session info, so the
  // phantom-spawn "durduruldu" badge appears/clears without waiting for a manual
  // refresh or the busy-poll (a halted coordinator is idle, so nothing else refetches).
  useEffect(() => {
    return subscribeWorkerChange(sessionId, () => setLocalRefresh((n) => n + 1))
  }, [sessionId])

  // While a turn is running (or a warm CLI process is held), poll the info endpoint
  // so the process card appears/updates/clears live even without a chat SSE bound to
  // this panel (e.g. an autonomous or detached turn). Light 3s cadence, no spinner.
  const isBusy = !!info?.running || !!info?.warmCliProcess
  useEffect(() => {
    if (!isBusy) return
    let alive = true
    const t = setInterval(() => {
      api
        .sessionInfo(sessionId)
        .then((d) => alive && setInfo(d))
        .catch(() => {})
    }, 3000)
    return () => {
      alive = false
      clearInterval(t)
    }
  }, [isBusy, sessionId])

  // Live 1s tick: drives the running-turn elapsed timer AND the prompt-cache
  // warmth countdown. Runs while a turn is in flight OR the cache is still warm,
  // then self-stops once it goes cold (no per-second effect churn — the gate
  // depends only on the stable running/updatedAt inputs).
  useEffect(() => {
    const running = !!info?.running
    const updatedAt = info?.updatedAt ?? 0
    const needsTick = () => running || cacheRemaining(updatedAt, serverNow()) > 0
    if (!needsTick()) return
    const t = setInterval(() => {
      setNowTick(serverNow())
      if (!needsTick()) clearInterval(t)
    }, 1000)
    return () => clearInterval(t)
  }, [info?.running, info?.updatedAt])

  // Stop the in-flight turn: cancels the run's context, which terminates the
  // background provider/claude-cli subprocess. Works for detached/autonomous turns.
  const handleStopProc = async () => {
    if (!info?.running || procBusy) return
    setProcBusy('stop')
    try {
      await api.chatControl(info.running.runId, 'stop')
      setInfo((prev) => (prev ? { ...prev, running: undefined } : prev))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setProcBusy('')
      setLocalRefresh((n) => n + 1)
    }
  }

  // Restart: delegate to the chat hook (stop in-flight + re-send the last user
  // prompt) so the turn goes through the one true streaming path.
  const handleRestartProc = async () => {
    if (!onRerun || procBusy) return
    setProcBusy('restart')
    try {
      // Stop the in-flight turn via the backend runId first (robust for detached /
      // autonomous turns this window doesn't own), then re-send the last prompt.
      if (info?.running) await api.chatControl(info.running.runId, 'stop').catch(() => {})
      await onRerun(sessionId)
      setInfo((prev) => (prev ? { ...prev, running: undefined } : prev))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setProcBusy('')
      setLocalRefresh((n) => n + 1)
    }
  }

  // Recycle the warm (persistent-pool) claude-cli process so the next turn cold-
  // restarts fresh. Conversation untouched.
  const handleDropProc = async () => {
    if (procBusy) return
    setProcBusy('drop')
    try {
      await api.dropSessionCliProcess(sessionId)
      setInfo((prev) => (prev ? { ...prev, warmCliProcess: false } : prev))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setProcBusy('')
      setLocalRefresh((n) => n + 1)
    }
  }

  // Regenerate the title, showing an inline spinner, then refresh the panel so
  // the new title is reflected here too.
  const handleTitle = async () => {
    if (titling) return
    setTitling(true)
    try {
      await onGenerateTitle(sessionId)
      setLocalRefresh((n) => n + 1)
    } finally {
      setTitling(false)
    }
  }

  // Toggle the session's archive state ("archived" drops it from the active
  // sidebar list without deleting; "active" restores it). Reflect locally and
  // refresh so the state pill updates.
  const handleArchiveToggle = async () => {
    if (archiving || !info) return
    const next = info.state === 'archived' ? 'active' : 'archived'
    setArchiving(true)
    try {
      await api.setSessionState(sessionId, next)
      setInfo((prev) => (prev ? { ...prev, state: next } : prev))
      setLocalRefresh((n) => n + 1)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setArchiving(false)
    }
  }

  // Pin / unpin from the title block. The parent handler keeps the sidebar list
  // in sync; without one (legacy usages) hit the API directly.
  const handlePinToggle = async () => {
    if (pinning || !info) return
    const next = !info.pinned
    setPinning(true)
    try {
      if (onSetPinned) onSetPinned(sessionId, next)
      else await api.setSessionPinned(sessionId, next)
      setInfo((prev) => (prev ? { ...prev, pinned: next } : prev))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setPinning(false)
    }
  }

  // Open the inline title editor seeded with the current title.
  const startEditTitle = () => {
    setTitleDraft(info?.title ?? '')
    setEditingTitle(true)
  }

  // Persist the manually edited title verbatim, then reflect it locally.
  const commitTitle = async () => {
    const t = titleDraft.trim()
    if (!t || t === info?.title) {
      setEditingTitle(false)
      return
    }
    setSavingTitle(true)
    try {
      await onRename(sessionId, t)
      setInfo((prev) => (prev ? { ...prev, title: t } : prev))
      setEditingTitle(false)
    } finally {
      setSavingTitle(false)
    }
  }

  // Context window figures (/context-style): used vs. the compaction threshold,
  // with the leftover shown as free space.
  // Stable projection target: an inline object literal would change identity on
  // every render, and the embedded ViewPanel refetches get_view whenever `target`
  // changes — so while the 1s timer/3s poll re-renders this panel, the projection
  // would refetch every tick. Memoize on sessionId so it only reloads on switch.
  const summaryTarget = useMemo(() => ({ kind: 'session' as const, id: sessionId }), [sessionId])

  const ctxWindow = info ? info.contextWindow || info.contextTokens || 1 : 1
  const ctxUsed = info ? info.contextTokens : 0
  const ctxFree = Math.max(0, ctxWindow - ctxUsed)
  const ctxPct = Math.round((ctxUsed / ctxWindow) * 100)

  return (
    <aside
      style={{ width }}
      className="th-col relative flex h-full shrink-0 flex-col overflow-hidden border-l border-[var(--color-border)] bg-[var(--color-surface)] max-md:!w-[85vw] max-md:!max-w-sm"
    >
      {/* Drag strip on the LEFT edge to resize the right-hand panel. It stays put
          while the content scrolls, so the scroll lives on the inner wrapper. */}
      <ResizeHandle onMouseDown={startDrag} side="left" />
      <div className="flex h-full flex-col overflow-y-auto">
        <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
          <div className="flex min-w-0 items-center gap-1.5">
            <span className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
              Oturum bilgisi
            </span>
            {/* Machine-written transcripts (insight scans, inbox, run logs) take
                no new user turns, so the composer is hidden. Say so here, or the
                missing composer looks like a bug. */}
            {info && !isWritableSessionKind(info.kind) && (
              <Badge tone="muted" className="shrink-0">
                Salt okunur
              </Badge>
            )}
          </div>
          <div className="flex items-center gap-1">
            <button
              onClick={() => setLocalRefresh((n) => n + 1)}
              disabled={loading}
              title="Yenile"
              className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-40"
            >
              <span className={`inline-block ${loading ? 'animate-spin' : ''}`}>↻</span>
            </button>
            <button
              onClick={onClose}
              title="Paneli kapat"
              className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              ✕
            </button>
          </div>
        </div>

        {loading && !info ? (
          <p className="px-4 py-6 text-sm text-[var(--color-text-dim)]">Yükleniyor…</p>
        ) : !info ? (
          <p className="px-4 py-6 text-sm text-[var(--color-text-dim)]">Bilgi yok.</p>
        ) : (
          <div className="flex flex-col gap-5 px-4 py-4">
            {/* Title + status */}
            <SessionTitleBlock
              info={info}
              editingTitle={editingTitle}
              titleDraft={titleDraft}
              savingTitle={savingTitle}
              setTitleDraft={setTitleDraft}
              setEditingTitle={setEditingTitle}
              startEditTitle={startEditTitle}
              commitTitle={commitTitle}
              onSelectSession={onSelectSession}
              onGenerateTitle={handleTitle}
              titling={titling}
            />

            {/* Background process: an in-flight turn and/or a warm persistent CLI
              process for this session — with stop / restart / recycle controls. */}
            {(info.running || info.warmCliProcess) && (
              <SessionProcessCard
                info={info}
                nowTick={nowTick}
                procBusy={procBusy}
                onStop={handleStopProc}
                onRestart={handleRestartProc}
                onDrop={handleDropProc}
                hasRerun={!!onRerun}
              />
            )}

            {/* Tags — free-form labels (also drive tag-triggered automations) */}
            <section>
              <div className="mb-2 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
                <span>Etiketler</span>
              </div>
              <TagEditor
                tags={info.tags ?? []}
                onChange={async (tags) => {
                  setInfo((prev) => (prev ? { ...prev, tags } : prev))
                  try {
                    await api.setSessionTags(sessionId, tags)
                  } catch {
                    setLocalRefresh((n) => n + 1) // reload on failure to resync
                  }
                }}
                placeholder="Etiket ekle (otomasyon tetikleyicisi olabilir)…"
              />
            </section>

            {/* Session projection ("Özet"): the same compact get_view output an
              agent receives. Moved here from the chat header's old ◱ Özet drawer;
              coordination now has its own "Coord" side sheet in the header. */}
            <section>
              <button
                onClick={toggleSummary}
                aria-expanded={summaryOpen}
                className="mb-2 flex w-full items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70 transition hover:text-[var(--color-accent)] hover:opacity-100"
              >
                {summaryOpen ? (
                  <ChevronDown size={12} className="shrink-0" />
                ) : (
                  <ChevronRight size={12} className="shrink-0" />
                )}
                <span>Özet</span>
              </button>
              {summaryOpen && (
                <div className="overflow-hidden rounded-lg border border-[var(--color-border)]">
                  <ViewPanel embedded target={summaryTarget} />
                </div>
              )}
            </section>

            {/* Meta */}
            <Section title="Genel">
              <Row label="Başlama" value={formatDate(info.createdAt)} />
              <Row label="Son etkinlik" value={formatDate(info.updatedAt)} />
              {/* Prompt-cache warmth: how long the cached prefix stays warm after
                the last turn (1h Anthropic ephemeral TTL / prompt epoch). */}
              <div className="flex items-center justify-between py-0.5 text-xs">
                <span className="text-[var(--color-text-dim)]">Prompt cache</span>
                <CacheWarmthBadge updatedAt={info.updatedAt} nowSec={nowTick} />
              </div>
              {info.model && (
                <div className="flex items-center justify-between py-0.5 text-xs">
                  <span className="text-[var(--color-text-dim)]">Model</span>
                  <span
                    className="rounded bg-[var(--color-surface-2)] px-1.5 py-px font-mono text-[var(--color-text-dim)]"
                    title={info.model}
                  >
                    {info.model}
                  </span>
                </div>
              )}
              <Row
                label="Boyut"
                value={`${formatBytes(info.sizeBytes)} · ${info.fileCount} dosya`}
              />
              <Row label="Mesaj sayısı" value={String(info.messageCount)} />
            </Section>

            {(info.executionType === 'subagent' || info.category === 'subagent') && (
              <SessionExecutionCard info={info} />
            )}

            {/* Context window usage (/context-style) */}
            <SessionContextUsage
              info={info}
              ctxWindow={ctxWindow}
              ctxUsed={ctxUsed}
              ctxFree={ctxFree}
              ctxPct={ctxPct}
            />

            {/* Agents */}
            <SessionAgentsSection info={info} onSelectAgent={onSelectAgent} />

            {/* This session's own lifetime spend + savings — the per-conversation
              cost (the session-scoped analog of the agent's daily total below). */}
            {sessionUsage && sessionUsage.calls > 0 && (
              <SessionUsageCard sessionUsage={sessionUsage} />
            )}

            {/* Debug / observability lives in its own panel (SessionDebugModal),
              opened from the "Debug" action at the bottom of this panel. */}

            {/* Actions / tools. AI title generation moved next to the title's edit
              control (SessionTitleBlock); "Bağlam" lives in the chat header. */}
            <Section title="Araçlar">
              <div className="flex flex-col gap-1.5">
                {onOpenDebug && (
                  <ActionBtn icon={Bug} label="Debug / gözlemlenebilirlik" onClick={onOpenDebug} />
                )}
                <ActionBtn
                  icon={info.pinned ? PinOff : Pin}
                  label={pinning ? '…' : info.pinned ? 'Sabitlemeyi kaldır' : 'Üste sabitle'}
                  onClick={handlePinToggle}
                  disabled={pinning}
                  busy={pinning}
                />
                <ActionBtn
                  icon={info.state === 'archived' ? ArchiveRestore : Archive}
                  label={
                    archiving ? '…' : info.state === 'archived' ? 'Arşivden kaldır' : 'Arşivle'
                  }
                  onClick={handleArchiveToggle}
                  disabled={archiving}
                  busy={archiving}
                />
                <ActionBtn
                  icon={Trash2}
                  label="Oturumu sil"
                  danger
                  onClick={() => {
                    if (confirm(`"${info.title || 'Bu oturum'}" silinsin mi?`))
                      onDeleteSession(sessionId)
                  }}
                />
              </div>
            </Section>
          </div>
        )}
      </div>
    </aside>
  )
}
