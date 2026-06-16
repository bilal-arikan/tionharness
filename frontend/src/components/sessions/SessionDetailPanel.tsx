import { useEffect, useState } from 'react'
import { api } from '../../api'
import type { SessionInfo } from '../../types'
import { AgentAvatar } from '../agents/AgentAvatar'

interface Props {
  sessionId: string
  // Bumped by the parent whenever the conversation changes, so size/context
  // figures refresh without reselecting the session.
  refreshKey?: number
  onClose: () => void
  onError: (msg: string) => void
  onCopyPath: (id: string) => void
  onRevealFolder: (id: string) => void
  onGenerateTitle: (id: string) => void | Promise<void>
  onSummarize: (id: string, kind: string) => void
  onDeleteSession: (id: string) => void
}

const SUMMARY_KINDS: { kind: string; label: string }[] = [
  { kind: 'memory', label: 'Hafıza' },
  { kind: 'board', label: 'Görev panosu' },
  { kind: 'flows', label: 'Akışlar' },
  { kind: 'tools', label: 'Araçlar' },
]

// SessionDetailPanel is the right-hand inspector for the active chat session:
// on-disk footprint, context composition, participating agents and quick actions.
export function SessionDetailPanel({
  sessionId,
  refreshKey,
  onClose,
  onError,
  onCopyPath,
  onRevealFolder,
  onGenerateTitle,
  onSummarize,
  onDeleteSession,
}: Props) {
  const [info, setInfo] = useState<SessionInfo | null>(null)
  const [loading, setLoading] = useState(false)
  const [copied, setCopied] = useState(false)
  const [summaryOpen, setSummaryOpen] = useState(false)
  const [titling, setTitling] = useState(false)
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

  const copyPath = () => {
    onCopyPath(sessionId)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
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
            <h3 className="truncate text-sm font-semibold text-[var(--color-text)]" title={info.title}>
              {info.title || 'Yeni sohbet'}
            </h3>
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              {info.state && <Pill>{info.state}</Pill>}
              {info.kind && <Pill>{info.kind}</Pill>}
              {info.unread && <Pill accent>okunmadı</Pill>}
            </div>
          </div>

          {/* Meta */}
          <Section title="Genel">
            <Row label="Başlama" value={formatDate(info.createdAt)} />
            <Row label="Son etkinlik" value={formatDate(info.updatedAt)} />
            <Row label="Mesaj sayısı" value={String(info.messageCount)} />
            <Row label="Boyut" value={`${formatBytes(info.sizeBytes)} · ${info.fileCount} dosya`} />
          </Section>

          {/* Folder */}
          <Section title="Klasör">
            <code className="block break-all rounded bg-[var(--color-bg)] px-2 py-1.5 font-mono text-[11px] text-[var(--color-text-dim)]">
              {info.path || '—'}
            </code>
            <div className="mt-2 flex gap-2">
              <SmallBtn onClick={copyPath}>{copied ? '✓ Kopyalandı' : '📋 Yolu kopyala'}</SmallBtn>
              <SmallBtn onClick={() => onRevealFolder(sessionId)}>📂 Aç</SmallBtn>
            </div>
          </Section>

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
              <p className="mt-1 text-[10px] text-amber-400">
                Pencere aşıldı — sonraki turda eski turlar özete sıkıştırılır.
              </p>
            )}
          </Section>

          {/* Agents */}
          <Section title={`Konuşmadaki ajanlar (${info.agents.length})`}>
            <div className="flex flex-col gap-2">
              {info.agents.map((a) => (
                <div key={a.agentId} className="flex items-center gap-2">
                  <AgentAvatar
                    agent={{ id: a.agentId, name: a.name, avatar: a.avatar, color: a.color }}
                    size={24}
                  />
                  <span className="flex min-w-0 flex-1 flex-col leading-tight">
                    <span className={`truncate text-xs ${a.disabled ? 'italic text-[var(--color-text-dim)]' : 'text-[var(--color-text)]'}`}>
                      {a.name}
                      {a.isOwner && <span className="ml-1 text-[var(--color-accent)]" title="Varsayılan ajan">★</span>}
                    </span>
                    <span className="text-[10px] text-[var(--color-text-dim)]">
                      {a.turns} tur · ~{formatTokens(a.tokens)} token
                    </span>
                  </span>
                </div>
              ))}
            </div>
          </Section>

          {/* Actions / tools */}
          <Section title="Araçlar">
            <div className="flex flex-col gap-1.5">
              <ActionBtn
                icon="✨"
                label={titling ? 'Başlık üretiliyor…' : 'AI ile başlık üret'}
                onClick={handleTitle}
                disabled={info.messageCount === 0 || titling}
                busy={titling}
              />
              <div className="relative">
                <ActionBtn icon="📝" label="Özet ekle…" onClick={() => setSummaryOpen((v) => !v)} caret />
                {summaryOpen && (
                  <div className="mt-1 flex flex-col gap-0.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-1">
                    {SUMMARY_KINDS.map((s) => (
                      <button
                        key={s.kind}
                        onClick={() => {
                          setSummaryOpen(false)
                          onSummarize(sessionId, s.kind)
                        }}
                        className="rounded px-2 py-1 text-left text-xs text-[var(--color-text)] transition hover:bg-[var(--color-surface-2)]"
                      >
                        {s.label}
                      </button>
                    ))}
                  </div>
                )}
              </div>
              <ActionBtn
                icon="🗑"
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
    </aside>
  )
}

// ---- presentational helpers ----

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

function SmallBtn({ children, onClick }: { children: React.ReactNode; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="flex-1 rounded-lg border border-[var(--color-border)] px-2 py-1.5 text-[11px] text-[var(--color-text)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
    >
      {children}
    </button>
  )
}

function ActionBtn({
  icon,
  label,
  onClick,
  disabled,
  danger,
  caret,
  busy,
}: {
  icon: string
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
          ? 'text-red-400 hover:bg-red-500/10'
          : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
      }`}
    >
      <span className={busy ? 'inline-block animate-spin' : ''}>{busy ? '↻' : icon}</span>
      <span className="flex-1">{label}</span>
      {caret && <span className="text-[var(--color-text-dim)]">▾</span>}
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
// usage bar and legend.
function fillerColor(role: string): string {
  switch (role) {
    case 'summary':
      return '#f59e0b' // amber — folded history
    case 'user':
      return 'var(--color-accent)'
    case 'assistant':
      return '#10b981' // emerald
    case 'tool':
      return '#8b5cf6' // violet
    case 'system':
      return '#64748b' // slate
    default:
      return '#94a3b8'
  }
}

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
