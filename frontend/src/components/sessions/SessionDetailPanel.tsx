import { useEffect, useState } from 'react'
import { Sparkles, Trash2, Loader2, ChevronDown, Check, Pencil, X, Target, CheckCircle2, Circle, ScanEye, PiggyBank, ListChecks, Square, ChevronRight, type LucideIcon } from 'lucide-react'
import { api } from '../../api'
import type { SessionInfo, SessionUsageDetail, SessionProgress } from '../../types'
import { SessionContextModal } from './SessionContextModal'
import { SessionDebugCard } from './SessionDebugCard'
import { AgentIdentity } from '../agents/AgentIdentity'
import { PromptEditor } from '../common'
import { roleColor } from '../../lib/palette'

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
}

function usd(n: number): string {
  if (n === 0) return '$0'
  if (n < 0.01) return `$${n.toFixed(4)}`
  return `$${n.toFixed(2)}`
}

function fmtTok(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return `${n}`
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
}: Props) {
  const [info, setInfo] = useState<SessionInfo | null>(null)
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
  // Debug: next-turn context preview modal visibility.
  const [ctxPreview, setCtxPreview] = useState(false)

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
    <aside className="flex h-full w-80 shrink-0 flex-col overflow-y-auto border-l border-[var(--color-border)] bg-[var(--color-surface)]">
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
              <p className="mb-2 text-[10px] text-[var(--color-text-dim)]">
                Bu sohbetin ömür boyu toplamı (yalnız bu oturum).
              </p>
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

          {/* Per-session debug journal (parallel observability stream): timings,
              token spend, tool latency/errors, compaction/recovery + raw log. */}
          <SessionDebugCard sessionId={sessionId} refreshKey={(refreshKey ?? 0) + localRefresh} />

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
              {/* Debug: preview the exact next-turn context the agent would get. */}
              <ActionBtn
                icon={ScanEye}
                label="Bağlam önizle (debug)"
                onClick={() => setCtxPreview(true)}
              />
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
      {ctxPreview && (
        <SessionContextModal
          sessionId={sessionId}
          title={info?.title}
          onClose={() => setCtxPreview(false)}
        />
      )}
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

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between py-0.5 text-xs">
      <span className="text-[var(--color-text-dim)]">{label}</span>
      <span className="text-[var(--color-text)]">{value}</span>
    </div>
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
