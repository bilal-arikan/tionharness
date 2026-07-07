import { useEffect, useState } from 'react'
import { Sparkles, Trash2, Loader2, ChevronDown, Check, Pencil, X, Target, CheckCircle2, Circle, PiggyBank, ListChecks, Square, ChevronRight, RotateCcw, Flame, type LucideIcon } from 'lucide-react'
import { api } from '../../api'
import type { SessionInfo, SessionUsageDetail, SessionProgress } from '../../types'
import { CoordinatorSection } from './CoordinatorSection'
import { AgentIdentity } from '../agents/AgentIdentity'
import { PromptEditor, KeyValueRow as Row, TagEditor } from '../common'
import { ResizeHandle } from '../common/SidebarChrome'
import { useResizableSidebar } from '../../hooks/useResizableSidebar'
import { roleColor } from '../../lib/palette'
import { usd, tokens as fmtTok } from '../../lib/format'

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
  onSelectSession,
  onSelectAgent,
  onRerun,
}: Props) {
  // Persisted, drag-resizable width. The panel sits on the RIGHT, so its handle
  // is on the LEFT edge and the drag direction is inverted (drag left = wider).
  const { width, startDrag } = useResizableSidebar({
    storageKey: 'tionswarm.sessionInfoWidth',
    defaultWidth: 320,
    min: 280,
    max: 640,
    invert: true,
  })
  const [info, setInfo] = useState<SessionInfo | null>(null)
  // In-flight action guard for the running-process card (stop/restart/drop).
  const [procBusy, setProcBusy] = useState<'' | 'stop' | 'restart' | 'drop'>('')
  // Ticks once a second while a turn is running, so the elapsed timer is live.
  const [nowTick, setNowTick] = useState(() => Math.floor(Date.now() / 1000))
  const [sessionUsage, setSessionUsage] = useState<SessionUsageDetail | null>(null)
  const [progress, setProgress] = useState<SessionProgress | null>(null)
  const [loading, setLoading] = useState(false)
  const [titling, setTitling] = useState(false)
  // Manual rename: when editing, hold the draft text; saving persists verbatim.
  const [editingTitle, setEditingTitle] = useState(false)
  const [titleDraft, setTitleDraft] = useState('')
  const [savingTitle, setSavingTitle] = useState(false)
  // Persistent goal ("north star"): when editing, hold the draft text; saving
  // persists verbatim (empty clears the goal).
  const [editingGoal, setEditingGoal] = useState(false)
  const [goalDraft, setGoalDraft] = useState('')
  const [savingGoal, setSavingGoal] = useState(false)
  // Manual-refresh nonce: bumped by the refresh button (and after a title
  // regeneration) to re-fetch without touching the parent's refreshKey.
  const [localRefresh, setLocalRefresh] = useState(0)

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

  // Persistent progress (durable todo_write checklist + log). Refetched on the
  // same triggers so a finished turn that updated the list reflects here.
  useEffect(() => {
    let alive = true
    api
      .sessionProgress(sessionId)
      .then((p) => alive && setProgress(p))
      .catch(() => alive && setProgress(null))
    return () => {
      alive = false
    }
  }, [sessionId, refreshKey, localRefresh])

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

  // Live elapsed timer: tick every second while a turn is running.
  useEffect(() => {
    if (!info?.running) return
    const t = setInterval(() => setNowTick(Math.floor(Date.now() / 1000)), 1000)
    return () => clearInterval(t)
  }, [info?.running])

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

  // Open the goal editor seeded with the current goal.
  const startEditGoal = () => {
    setGoalDraft(info?.goal ?? '')
    setEditingGoal(true)
  }

  // Persist the goal verbatim (empty clears it), then reflect it locally. Saving
  // always reopens the goal (done=false) — an edited objective is active again.
  const commitGoal = async () => {
    const g = goalDraft.trim()
    if (g === (info?.goal ?? '').trim() && !info?.goalDone) {
      setEditingGoal(false)
      return
    }
    setSavingGoal(true)
    try {
      await api.setSessionGoal(sessionId, g, false)
      setInfo((prev) => (prev ? { ...prev, goal: g, goalDone: false } : prev))
      setEditingGoal(false)
      // Re-fetch so the context meter reflects the goal's new footprint.
      setLocalRefresh((n) => n + 1)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSavingGoal(false)
    }
  }

  // Toggle the goal's done state: completing it keeps the text but stops the
  // context injection; reopening resumes it.
  const toggleGoalDone = async () => {
    if (!info?.goal || savingGoal) return
    const next = !info.goalDone
    setSavingGoal(true)
    try {
      await api.setSessionGoal(sessionId, info.goal, next)
      setInfo((prev) => (prev ? { ...prev, goalDone: next } : prev))
      // Re-fetch so the context meter drops/restores the goal bucket.
      setLocalRefresh((n) => n + 1)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSavingGoal(false)
    }
  }

  // Context window figures (/context-style): used vs. the compaction threshold,
  // with the leftover shown as free space.
  const ctxWindow = info ? info.contextWindow || info.contextTokens || 1 : 1
  const ctxUsed = info ? info.contextTokens : 0
  const ctxFree = Math.max(0, ctxWindow - ctxUsed)
  const ctxPct = Math.round((ctxUsed / ctxWindow) * 100)

  return (
    <aside
      style={{ width }}
      className="relative flex h-full shrink-0 flex-col overflow-hidden border-l border-[var(--color-border)] bg-[var(--color-surface)] max-md:!w-[85vw] max-md:!max-w-sm"
    >
      {/* Drag strip on the LEFT edge to resize the right-hand panel. It stays put
          while the content scrolls, so the scroll lives on the inner wrapper. */}
      <ResizeHandle onMouseDown={startDrag} side="left" />
      <div className="flex h-full flex-col overflow-y-auto">
      <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
        <span className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Oturum bilgisi
        </span>
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
          <div>
            {editingTitle ? (
              <div className="flex items-center gap-1.5">
                <input
                  autoFocus
                  value={titleDraft}
                  onChange={(e) => setTitleDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') commitTitle()
                    else if (e.key === 'Escape') setEditingTitle(false)
                  }}
                  disabled={savingTitle}
                  placeholder="Sohbet başlığı"
                  className="min-w-0 flex-1 rounded border border-[var(--color-accent)] bg-[var(--color-bg)] px-2 py-1 text-sm font-semibold text-[var(--color-text)] outline-none disabled:opacity-50"
                />
                <button
                  onClick={commitTitle}
                  disabled={savingTitle}
                  title="Kaydet"
                  className="rounded p-1 text-[var(--color-accent)] transition hover:opacity-80 disabled:opacity-40"
                >
                  {savingTitle ? <Loader2 size={15} className="animate-spin" /> : <Check size={15} />}
                </button>
                <button
                  onClick={() => setEditingTitle(false)}
                  disabled={savingTitle}
                  title="İptal"
                  className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-text)] disabled:opacity-40"
                >
                  <X size={15} />
                </button>
              </div>
            ) : (
              <div className="group flex items-center gap-1.5">
                <h3 className="min-w-0 flex-1 truncate text-sm font-semibold text-[var(--color-text)]" title={info.title}>
                  {info.title || 'Yeni sohbet'}
                </h3>
                <button
                  onClick={startEditTitle}
                  title="Başlığı düzenle"
                  className="shrink-0 rounded p-1 text-[var(--color-text-dim)] opacity-0 transition hover:text-[var(--color-accent)] group-hover:opacity-100"
                >
                  <Pencil size={13} />
                </button>
              </div>
            )}
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              {info.state && <Pill>{info.state}</Pill>}
              {info.kind && <Pill>{info.kind}</Pill>}
              {info.unread && <Pill accent>okunmadı</Pill>}
            </div>
            {/* Context-reset lineage: this session continues an earlier one. */}
            {info.parentSessionId && (
              <button
                type="button"
                onClick={() => onSelectSession?.(info.parentSessionId!)}
                disabled={!onSelectSession}
                className="mt-1.5 inline-flex items-center gap-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-text)] disabled:cursor-default disabled:hover:text-[var(--color-text-dim)]"
                title="Bu oturum bir context reset (handoff) ile önceki oturumdan devraldı"
              >
                ↩ Devraldığı oturum: <span className="font-mono">{info.parentSessionId}</span>
              </button>
            )}
          </div>

          {/* Background process: an in-flight turn and/or a warm persistent CLI
              process for this session — with stop / restart / recycle controls. */}
          {(info.running || info.warmCliProcess) && (
            <section className="flex flex-col gap-2 rounded-lg border border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-accent)_6%,transparent)] px-2.5 py-2">
              {info.running && (
                <>
                  <div className="flex items-center gap-1.5 text-[11px]">
                    <Loader2 size={13} className="shrink-0 animate-spin text-[var(--color-accent)]" />
                    <span className="font-medium text-[var(--color-text)]">
                      {info.running.autonomous ? 'Otonom tur çalışıyor' : 'Tur çalışıyor'}
                    </span>
                    {info.running.provider && (
                      <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-text-dim)]">
                        {info.running.provider}
                      </span>
                    )}
                    <span className="ml-auto font-mono text-[10px] text-[var(--color-text-dim)]">
                      {formatElapsed(Math.max(0, nowTick - info.running.startedAt))}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <ProcBtn
                      icon={Square}
                      label="Durdur"
                      onClick={handleStopProc}
                      busy={procBusy === 'stop'}
                      disabled={procBusy !== ''}
                    />
                    {onRerun && (
                      <ProcBtn
                        icon={RotateCcw}
                        label="Yeniden başlat"
                        onClick={handleRestartProc}
                        busy={procBusy === 'restart'}
                        disabled={procBusy !== ''}
                      />
                    )}
                  </div>
                </>
              )}
              {info.warmCliProcess && (
                <div className="flex items-center gap-1.5">
                  <Flame size={13} className="shrink-0 text-[var(--color-warning)]" />
                  <span className="min-w-0 flex-1 text-[11px] text-[var(--color-text-dim)]">
                    Sıcak claude-cli süreci (turlar arası)
                  </span>
                  <ProcBtn
                    icon={RotateCcw}
                    label="Tazele"
                    onClick={handleDropProc}
                    busy={procBusy === 'drop'}
                    disabled={procBusy !== ''}
                    title="Sıcak süreci kapat — sonraki tur temiz başlar (konuşma korunur)"
                  />
                </div>
              )}
            </section>
          )}

          {/* Goal ("north star") — persistent objective injected into context */}
          <section>
            <div className="mb-2 flex items-center justify-between">
              <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
                <Target size={12} className="shrink-0" />
                <span>Hedef</span>
              </div>
              {!editingGoal && (
                <button
                  onClick={startEditGoal}
                  title={info.goal ? 'Hedefi düzenle' : 'Hedef belirle'}
                  className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                >
                  <Pencil size={13} />
                </button>
              )}
            </div>
            {editingGoal ? (
              <div className="flex flex-col gap-1.5">
                <PromptEditor
                  autoFocus
                  value={goalDraft}
                  onChange={setGoalDraft}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) commitGoal()
                    else if (e.key === 'Escape') setEditingGoal(false)
                  }}
                  disabled={savingGoal}
                  rows={4}
                  maxLength={2000}
                  placeholder="Bu sohbet için kalıcı bir hedef yaz — ajan her turda buna göre ilerler. Örn: 'X özelliğini test ederek bitir ve PR aç.'"
                  className="border-[var(--color-accent)]"
                  textareaClassName="text-xs"
                />
                <div className="flex items-center gap-2">
                  <button
                    onClick={commitGoal}
                    disabled={savingGoal}
                    className="flex items-center gap-1.5 rounded-lg bg-[var(--color-accent)] px-2.5 py-1.5 text-[11px] font-medium text-white transition hover:opacity-90 disabled:opacity-40"
                  >
                    {savingGoal ? <Loader2 size={13} className="animate-spin" /> : <Check size={13} />}
                    Kaydet
                  </button>
                  <button
                    onClick={() => setEditingGoal(false)}
                    disabled={savingGoal}
                    className="rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-[11px] text-[var(--color-text-dim)] transition hover:text-[var(--color-text)] disabled:opacity-40"
                  >
                    İptal
                  </button>
                  <span className="ml-auto text-[10px] text-[var(--color-text-dim)]">⌘/Ctrl+Enter</span>
                </div>
              </div>
            ) : info.goal ? (
              <div className="flex flex-col gap-1.5">
                <p
                  className={`whitespace-pre-wrap rounded-lg border px-2.5 py-2 text-xs leading-relaxed ${
                    info.goalDone
                      ? 'border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-success)_8%,transparent)] text-[var(--color-text-dim)] line-through decoration-[var(--color-text-dim)]/60'
                      : 'border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-accent)_8%,transparent)] text-[var(--color-text)]'
                  }`}
                >
                  {info.goal}
                </p>
                <div className="flex items-center justify-between">
                  <button
                    onClick={toggleGoalDone}
                    disabled={savingGoal}
                    title={info.goalDone ? 'Hedefi yeniden aç (context\'e tekrar enjekte edilir)' : 'Tamamlandı olarak işaretle (context enjeksiyonu durur)'}
                    className={`flex items-center gap-1.5 rounded-lg border px-2 py-1 text-[11px] transition disabled:opacity-40 ${
                      info.goalDone
                        ? 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                        : 'border-[var(--color-success)]/40 text-[var(--color-success)] hover:bg-[var(--color-success)]/10'
                    }`}
                  >
                    {savingGoal ? (
                      <Loader2 size={13} className="animate-spin" />
                    ) : info.goalDone ? (
                      <Circle size={13} />
                    ) : (
                      <CheckCircle2 size={13} />
                    )}
                    {info.goalDone ? 'Yeniden aç' : 'Tamamlandı'}
                  </button>
                  {info.goalDone && (
                    <span className="flex items-center gap-1 text-[10px] font-medium text-[var(--color-success)]">
                      <CheckCircle2 size={12} /> Tamamlandı · enjekte edilmiyor
                    </span>
                  )}
                </div>
              </div>
            ) : (
              <button
                onClick={startEditGoal}
                className="flex w-full items-center gap-2 rounded-lg border border-dashed border-[var(--color-border)] px-2.5 py-2 text-left text-[11px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              >
                <Target size={13} className="shrink-0" />
                Bu sohbet için bir hedef belirle
              </button>
            )}
          </section>

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

          {/* Coordinator/worker (M2): toggle coordinator mode + live worker roster */}
          <CoordinatorSection
            sessionId={sessionId}
            role={info.role}
            refreshKey={(refreshKey ?? 0) + localRefresh}
            onError={onError}
            onRoleChanged={() => setLocalRefresh((n) => n + 1)}
          />

          {/* Meta */}
          <Section title="Genel">
            <Row label="Başlama" value={formatDate(info.createdAt)} />
            <Row label="Son etkinlik" value={formatDate(info.updatedAt)} />
            <Row label="Mesaj sayısı" value={String(info.messageCount)} />
            <Row label="Boyut" value={`${formatBytes(info.sizeBytes)} · ${info.fileCount} dosya`} />
          </Section>


          {/* Persistent progress (durable todo_write checklist + rolling log) */}
          {progress?.exists && progress.record && progress.record.todos.length > 0 && (
            <ProgressCard progress={progress} />
          )}

          {/* Context window usage (/context-style) */}
          <Section title={`Bağlam penceresi · ${formatTokens(ctxUsed)}/${formatTokens(ctxWindow)} (${ctxPct}%)`}>
            {/* Stacked usage bar: each filler a coloured segment, remainder free. */}
            <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-[var(--color-bg)]">
              {info.fillers.map((f) => (
                <div
                  key={f.role}
                  title={`${f.label}: ~${formatTokens(f.tokens)}`}
                  style={{ width: `${(f.tokens / ctxWindow) * 100}%`, backgroundColor: fillerColor(f.role) }}
                />
              ))}
            </div>

            <div className="mt-2.5 flex flex-col gap-1">
              {info.fillers.map((f) => (
                <div key={f.role} className="flex items-center gap-2 text-[11px]">
                  <span className="h-2.5 w-2.5 shrink-0 rounded-sm" style={{ backgroundColor: fillerColor(f.role) }} />
                  <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
                    {f.label}
                    {f.count > 1 && <span className="text-[var(--color-text-dim)]"> ·{f.count}</span>}
                  </span>
                  <span className="shrink-0 font-mono text-[var(--color-text-dim)]">
                    ~{formatTokens(f.tokens)} · {pctOf(f.tokens, ctxWindow)}%
                  </span>
                </div>
              ))}
              {/* Free space */}
              <div className="flex items-center gap-2 text-[11px]">
                <span className="h-2.5 w-2.5 shrink-0 rounded-sm border border-[var(--color-border)] bg-[var(--color-bg)]" />
                <span className="min-w-0 flex-1 text-[var(--color-text-dim)]">Boş alan</span>
                <span className="shrink-0 font-mono text-[var(--color-text-dim)]">
                  ~{formatTokens(ctxFree)} · {pctOf(ctxFree, ctxWindow)}%
                </span>
              </div>
            </div>

            {info.hasSummary && (
              <p className="mt-2 text-[10px] text-[var(--color-text-dim)]">
                İlk {info.summaryMsgCount} mesaj özete katlandı (~{formatTokens(info.summaryTokens)} token).
              </p>
            )}
            {ctxUsed > ctxWindow && (
              <p className="mt-1 text-[10px] text-[var(--color-warning)]">
                Pencere aşıldı — sonraki turda eski turlar özete sıkıştırılır.
              </p>
            )}
          </Section>

          {/* Agents */}
          <Section title={`Konuşmadaki ajanlar (${info.agents.length})`}>
            <div className="flex flex-col gap-2">
              {info.agents.map((a) => {
                const row = (
                  <AgentIdentity
                    agent={{ id: a.agentId, name: a.name, avatar: a.avatar, color: a.color }}
                    size="sm"
                    dim={a.disabled}
                    nameSuffix={
                      a.isOwner ? (
                        <span className="ml-1 text-[var(--color-accent)]" title="Varsayılan ajan">
                          ★
                        </span>
                      ) : undefined
                    }
                    subtitle={`${a.turns} tur · ~${formatTokens(a.tokens)} token`}
                  />
                )
                // Without a handler, render the bare row (read-only). With one,
                // wrap in a button that navigates to the Agents view focused on
                // that agent — matching the behaviour of clicking an agent in the
                // Agents roster. Disabled agents stay non-interactive.
                if (!onSelectAgent || a.disabled) return <div key={a.agentId}>{row}</div>
                return (
                  <button
                    key={a.agentId}
                    type="button"
                    onClick={() => onSelectAgent(a.agentId)}
                    title={`${a.name} sayfasına git`}
                    className="group flex w-full items-center justify-between rounded-md border border-transparent px-2 py-1.5 text-left transition hover:border-[var(--color-border)] hover:bg-[var(--color-surface-hover)] focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)]"
                  >
                    <span className="min-w-0 flex-1">{row}</span>
                    <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--color-text-dim)] opacity-0 transition group-hover:opacity-100" aria-hidden />
                  </button>
                )
              })}
            </div>
          </Section>

          {/* This session's own lifetime spend + savings — the per-conversation
              cost (the session-scoped analog of the agent's daily total below). */}
          {sessionUsage && (sessionUsage.calls > 0 || sessionUsage.compactSavedBytes > 0 || sessionUsage.compactSavedBytesLLM > 0) && (
            <Section title="Bu oturumun harcaması">
              <div className="mb-2 flex items-baseline gap-2">
                <span className="text-lg font-semibold text-[var(--color-text)]">
                  {(sessionUsage.estimated ? '~' : '') + usd(sessionUsage.costUSD)}
                </span>
                <span className="text-[10px] text-[var(--color-text-dim)]">
                  {sessionUsage.calls} çağrı · {fmtTok(sessionUsage.inputTokens + sessionUsage.outputTokens)} token
                </span>
              </div>
              {/* Savings breakdown: prompt-cache USD + tool-output compaction bytes */}
              <div className="flex flex-col gap-1 rounded-lg border border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-success)_6%,transparent)] px-2.5 py-2">
                <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-success)]">
                  <PiggyBank size={12} /> Kazanç / tasarruf
                </div>
                {sessionUsage.savingsUSD > 0 && (
                  <SaveRow label="Prompt-cache" value={usd(sessionUsage.savingsUSD)} />
                )}
                {sessionUsage.compactSavedBytes > 0 && (
                  <SaveRow label="Sıkıştırma (kural)" value={formatBytes(sessionUsage.compactSavedBytes)} hint="araç çıktısından kırpılan" />
                )}
                {sessionUsage.compactSavedBytesLLM > 0 && (
                  <SaveRow label="Sıkıştırma (LLM)" value={formatBytes(sessionUsage.compactSavedBytesLLM)} hint="özetle kırpılan" />
                )}
                {sessionUsage.savingsUSD === 0 && sessionUsage.compactSavedBytes === 0 && sessionUsage.compactSavedBytesLLM === 0 && (
                  <span className="text-[11px] text-[var(--color-text-dim)]">Henüz tasarruf yok.</span>
                )}
              </div>
            </Section>
          )}

          {/* Debug / observability moved to its own panel — opened from the chat
              header's "Debug" button (SessionDebugModal). */}

          {/* Actions / tools */}
          <Section title="Araçlar">
            <div className="flex flex-col gap-1.5">
              <ActionBtn
                icon={Sparkles}
                label={titling ? 'Başlık üretiliyor…' : 'AI ile başlık üret'}
                onClick={handleTitle}
                disabled={info.messageCount === 0 || titling}
                busy={titling}
              />
              {/* "Bağlam" moved to the chat header (App.tsx) so it opens
                  without first opening this inspector. */}
              <ActionBtn
                icon={Trash2}
                label="Oturumu sil"
                danger
                onClick={() => {
                  if (confirm(`"${info.title || 'Bu oturum'}" silinsin mi?`)) onDeleteSession(sessionId)
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

// ---- presentational helpers ----

// ProgressCard renders the session's persistent progress file read-only: a count
// summary, each checklist item with its status marker (and optional category),
// and the most recent rolling-log lines. Surfaces the cross-session note-taking
// that the agent maintains via todo_write.
function ProgressCard({ progress }: { progress: SessionProgress }) {
  const rec = progress.record!
  const total = rec.todos.length
  const done = rec.todos.filter((t) => t.status === 'completed').length
  const log = (rec.log ?? []).slice(-3).reverse()
  return (
    <section>
      <div className="mb-2 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
        <ListChecks size={12} className="shrink-0" />
        <span>Kalıcı ilerleme · {done}/{total}</span>
      </div>
      <div className="flex flex-col gap-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-2">
        {rec.todos.map((t, i) => (
          <div key={i} className="flex items-start gap-1.5 text-[11px] leading-relaxed">
            {t.status === 'completed' ? (
              <CheckCircle2 size={13} className="mt-px shrink-0" style={{ color: 'var(--color-success)' }} />
            ) : t.status === 'in_progress' ? (
              <Loader2 size={13} className="mt-px shrink-0 text-[var(--color-accent)]" />
            ) : (
              <Square size={13} className="mt-px shrink-0 text-[var(--color-text-dim)]" />
            )}
            <span className={t.status === 'completed' ? 'text-[var(--color-text-dim)] line-through' : 'text-[var(--color-text)]'}>
              {t.content}
              {t.category && <span className="ml-1 opacity-50">· {t.category}</span>}
            </span>
          </div>
        ))}
      </div>
      {log.length > 0 && (
        <div className="mt-1.5 flex flex-col gap-0.5">
          {log.map((l, i) => (
            <div key={i} className="truncate text-[10px] text-[var(--color-text-dim)] opacity-70" title={l.note}>
              {l.note}
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section>
      <div className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
        {title}
      </div>
      {children}
    </section>
  )
}

// SaveRow is one savings line in the session spend card: a label, an optional
// hint, and a green value (USD or bytes).
function SaveRow({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="flex items-center justify-between text-[11px]">
      <span className="text-[var(--color-text-dim)]">
        {label}
        {hint && <span className="ml-1 opacity-60">· {hint}</span>}
      </span>
      <span className="ml-2 shrink-0 font-medium" style={{ color: 'var(--color-success)' }}>
        {value}
      </span>
    </div>
  )
}

// ProcBtn is a compact control in the background-process card (stop/restart/drop).
function ProcBtn({
  icon: Icon,
  label,
  onClick,
  busy,
  disabled,
  title,
}: {
  icon: LucideIcon
  label: string
  onClick: () => void
  busy?: boolean
  disabled?: boolean
  title?: string
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={title ?? label}
      className="flex items-center gap-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-[11px] text-[var(--color-text)] transition hover:border-[var(--color-accent)] disabled:opacity-40"
    >
      {busy ? <Loader2 size={12} className="shrink-0 animate-spin" /> : <Icon size={12} className="shrink-0" />}
      {label}
    </button>
  )
}

function Pill({ children, accent }: { children: React.ReactNode; accent?: boolean }) {
  return (
    <span
      className={`rounded-full px-2 py-0.5 text-[10px] ${
        accent
          ? 'bg-[var(--color-accent)]/15 text-[var(--color-accent)]'
          : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
      }`}
    >
      {children}
    </span>
  )
}

function ActionBtn({
  icon: Icon,
  label,
  onClick,
  disabled,
  danger,
  caret,
  busy,
}: {
  icon: LucideIcon
  label: string
  onClick: () => void
  disabled?: boolean
  danger?: boolean
  caret?: boolean
  busy?: boolean
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-xs transition disabled:opacity-30 ${
        danger
          ? 'text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]'
          : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
      }`}
    >
      {busy ? <Loader2 size={14} className="shrink-0 animate-spin" /> : <Icon size={14} className="shrink-0" />}
      <span className="flex-1">{label}</span>
      {caret && <ChevronDown size={14} className="text-[var(--color-text-dim)]" />}
    </button>
  )
}

// ---- formatting ----

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB']
  let v = bytes / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`
}

function formatTokens(t: number): string {
  if (t < 1000) return String(t)
  return `${(t / 1000).toFixed(1)}k`
}

// formatElapsed renders a running duration in seconds as "42sn" / "3d 5sn".
function formatElapsed(sec: number): string {
  if (sec < 60) return `${sec}sn`
  const m = Math.floor(sec / 60)
  const s = sec % 60
  if (m < 60) return `${m}d ${s}sn`
  const h = Math.floor(m / 60)
  return `${h}s ${m % 60}d`
}

// pctOf returns n as a whole-number percent of total (0 when total is 0).
function pctOf(n: number, total: number): number {
  return total > 0 ? Math.round((n / total) * 100) : 0
}

// fillerColor maps a context bucket role to a stable segment colour for the
// usage bar and legend (hues live in the shared categorical palette).
const fillerColor = roleColor

function formatDate(unixSec: number): string {
  if (!unixSec) return '—'
  return new Date(unixSec * 1000).toLocaleString('tr-TR', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}
