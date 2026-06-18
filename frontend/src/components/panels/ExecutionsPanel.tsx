import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  MessageSquare,
  LayoutGrid,
  GitBranch,
  Clock,
  Heart,
  Activity,
  RefreshCw,
  Copy,
  Loader2,
  Sparkles,
  type LucideIcon,
} from 'lucide-react'
import type { Agent, Execution, Message } from '../../types'
import { api } from '../../api'
import { MessageList } from '../chat/MessageList'
import { AgentAvatar } from '../agents/AgentAvatar'
import { SpawnSessionModal } from '../sessions/SpawnSessionModal'
import { relativeTime } from '../../lib/time'

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
  onOpenFile?: (path: string) => void
  onOpenArtifact?: (id: string) => void
  // Jump to the Flows screen on this flow's run history (flow executions only).
  onOpenFlowRun?: (flowId: string) => void
  // Deep-link: pre-select this execution (sessionId) on mount/route change.
  focusId?: string | null
  // Report the active selection up so the URL hash stays in sync.
  onSelectExecution?: (sessionId: string) => void
}

// Per-kind display metadata: every execution path funnels into a Session tagged
// with a kind, so the feed renders each uniformly with its own icon + label.
const KIND_META: Record<string, { label: string; icon: LucideIcon }> = {
  chat: { label: 'Sohbet', icon: MessageSquare },
  task: { label: 'Görev', icon: LayoutGrid },
  flow: { label: 'Akış', icon: GitBranch },
  schedule: { label: 'Zamanlama', icon: Clock },
  heartbeat: { label: 'Nabız', icon: Heart },
  spawned: { label: 'Spawn', icon: Sparkles },
}

// Filter tabs (in display order). '' is "all".
const FILTERS: { key: string; label: string }[] = [
  { key: '', label: 'Tümü' },
  { key: 'chat', label: 'Sohbet' },
  { key: 'task', label: 'Görev' },
  { key: 'flow', label: 'Akış' },
  { key: 'spawned', label: 'Spawn' },
  { key: 'schedule', label: 'Zamanlama' },
  { key: 'heartbeat', label: 'Nabız' },
]

const POLL_MS = 5000
// Faster polling interval used when the selected execution is still running.
const RUNNING_POLL_MS = 2000

function kindMeta(kind: string) {
  return KIND_META[kind] ?? { label: kind || 'Diğer', icon: Activity }
}

// shortId trims a session id to a compact, recognisable suffix for list rows
// (the full id is shown — and copyable — in the detail header).
function shortId(id: string) {
  return id.length > 8 ? id.slice(-8) : id
}

// StatusPill shows a finished run's pass/fail outcome (task/flow kinds).
function StatusPill({ status }: { status: string }) {
  if (status !== 'success' && status !== 'failure') return null
  const ok = status === 'success'
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-semibold"
      style={{
        color: ok ? 'var(--color-success)' : 'var(--color-danger)',
        backgroundColor: ok
          ? 'color-mix(in srgb, var(--color-success) 14%, transparent)'
          : 'color-mix(in srgb, var(--color-danger) 14%, transparent)',
      }}
    >
      {ok ? 'başarılı' : 'hata'}
    </span>
  )
}

// ExecutionsPanel is the unified activity feed: a single list of every execution
// across chat / task / flow / schedule / heartbeat (each backed by a Session),
// with live status, plus a read-only transcript viewer for the selected one.
export function ExecutionsPanel({ agents, onError, onOpenFile, onOpenArtifact, onOpenFlowRun, focusId, onSelectExecution }: Props) {
  const [items, setItems] = useState<Execution[]>([])
  const [filter, setFilter] = useState('')
  const [selectedId, setSelectedId] = useState<string | null>(focusId ?? null)
  const [messages, setMessages] = useState<Message[]>([])
  const [loading, setLoading] = useState(false)
  const [spawnOpen, setSpawnOpen] = useState(false)
  const selectedRef = useRef<string | null>(null)
  selectedRef.current = selectedId

  // Select an execution and mirror it to the URL (deep-link aware).
  const select = useCallback(
    (sessionId: string) => {
      setSelectedId(sessionId)
      onSelectExecution?.(sessionId)
    },
    [onSelectExecution],
  )

  // Honor an incoming deep-link (schedule/flow notification click): switch the
  // selection to the routed run when focusId changes.
  useEffect(() => {
    if (focusId) setSelectedId(focusId)
  }, [focusId])

  const load = useCallback(
    (kind: string) => {
      api
        .listExecutions(kind || undefined)
        .then(setItems)
        .catch((e) => onError((e as Error).message))
    },
    [onError],
  )

  // Initial + filter-change load, then poll so live "running" / status stays fresh.
  useEffect(() => {
    load(filter)
    const t = setInterval(() => load(filter), POLL_MS)
    return () => clearInterval(t)
  }, [filter, load])

  // Load the selected execution's transcript on selection change.
  useEffect(() => {
    if (!selectedId) {
      setMessages([])
      return
    }
    setLoading(true)
    api
      .listMessages(selectedId)
      .then((m) => {
        if (selectedRef.current === selectedId) setMessages(m)
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
  }, [selectedId, onError])

  const selected = useMemo(
    () => items.find((i) => i.sessionId === selectedId) ?? null,
    [items, selectedId],
  )

  // While the selected execution is running, re-fetch messages at a faster rate
  // so new turns appear in the transcript without waiting for the next list poll.
  const selectedRunning = selected?.running ?? false
  useEffect(() => {
    if (!selectedId || !selectedRunning) return
    const t = setInterval(() => {
      api
        .listMessages(selectedId)
        .then((m) => {
          if (selectedRef.current === selectedId) setMessages(m)
        })
        .catch(() => { /* best-effort */ })
    }, RUNNING_POLL_MS)
    return () => clearInterval(t)
  }, [selectedId, selectedRunning])

  return (
    <div className="flex h-full min-h-0">
      {/* Master: the executions list */}
      <aside className="flex h-full w-80 shrink-0 flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)]">
        <div className="flex items-center justify-between px-4 pt-4 pb-2">
          <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
            Yürütmeler
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setSpawnOpen(true)}
              title="Yeni oturum başlat (spawn)"
              className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-0.5 text-[11px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
            >
              <Sparkles size={12} /> Başlat
            </button>
            <button
              onClick={() => load(filter)}
              title="Yenile"
              className="text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              <RefreshCw size={14} />
            </button>
          </div>
        </div>

        {/* Kind filter tabs */}
        <div className="flex flex-wrap gap-1 px-3 pb-2">
          {FILTERS.map((f) => (
            <button
              key={f.key}
              onClick={() => setFilter(f.key)}
              className={`rounded-full px-2.5 py-1 text-[11px] transition ${
                filter === f.key
                  ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>

        <div className="flex-1 overflow-y-auto px-2 pb-2">
          {items.map((it) => {
            const meta = kindMeta(it.kind)
            const Icon = meta.icon
            const owner = agents.find((a) => a.id === it.agentId)
            const isActive = selectedId === it.sessionId
            return (
              <button
                key={it.sessionId}
                onClick={() => select(it.sessionId)}
                className={`mb-0.5 flex w-full items-start gap-2 rounded-lg px-3 py-2 text-left text-sm transition ${
                  isActive
                    ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
                    : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
                }`}
              >
                {owner ? (
                  <AgentAvatar agent={owner} size={20} />
                ) : (
                  <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-[var(--color-surface-2)]">
                    <Icon size={12} />
                  </span>
                )}
                <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="flex items-center gap-1.5">
                    {it.running ? (
                      <span className="relative flex h-2 w-2 shrink-0" title="Çalışıyor">
                        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--color-success)] opacity-75" />
                        <span className="relative inline-flex h-2 w-2 rounded-full bg-[var(--color-success)]" />
                      </span>
                    ) : (
                      it.unread && (
                        <span className="h-2 w-2 shrink-0 rounded-full bg-[var(--color-accent)]" title="Okunmadı" />
                      )
                    )}
                    <span
                      className={`min-w-0 flex-1 truncate ${
                        it.unread || it.running ? 'font-semibold text-[var(--color-text)]' : ''
                      }`}
                    >
                      {it.title || meta.label}
                    </span>
                  </span>
                  <span className="flex items-center gap-1.5 text-[10px] opacity-70">
                    <Icon size={11} className="shrink-0" />
                    <span>{meta.label}</span>
                    {it.agentName && <span>· {it.agentName}</span>}
                    <span>· {relativeTime(it.updatedAt)}</span>
                  </span>
                  <span className="font-mono text-[10px] text-[var(--color-text-dim)] opacity-60">
                    #{shortId(it.sessionId)}
                  </span>
                </span>
                {!it.running && <StatusPill status={it.lastStatus ?? ''} />}
              </button>
            )
          })}
          {items.length === 0 && (
            <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
              Henüz yürütme yok. Bir sohbet, görev, akış veya zamanlama çalıştığında burada belirir.
            </p>
          )}
        </div>
      </aside>

      {/* Detail: the selected execution's transcript (read-only) */}
      <div className="flex h-full min-w-0 flex-1 flex-col">
        {selected ? (
          <>
            <header className="flex items-center gap-2 border-b border-[var(--color-border)] px-6 py-3">
              {(() => {
                const Icon = kindMeta(selected.kind).icon
                return <Icon size={16} className="text-[var(--color-text-dim)]" />
              })()}
              <span className="truncate text-sm font-semibold">
                {selected.title || kindMeta(selected.kind).label}
              </span>
              <span className="text-xs text-[var(--color-text-dim)]">
                · {kindMeta(selected.kind).label}
                {selected.agentName ? ` · ${selected.agentName}` : ''}
              </span>
              <button
                type="button"
                onClick={() => navigator.clipboard?.writeText(selected.sessionId).catch(() => {})}
                title={`Kimliği kopyala: ${selected.sessionId}`}
                className="flex items-center gap-1 rounded border border-[var(--color-border)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              >
                <Copy size={10} /> #{shortId(selected.sessionId)}
              </button>
              {selected.running && (
                <span className="ml-1 text-[11px] font-medium text-[var(--color-success)]">çalışıyor…</span>
              )}
              {selected.kind === 'flow' && selected.sourceId && onOpenFlowRun && (
                <button
                  onClick={() => onOpenFlowRun(selected.sourceId!)}
                  title="Bu akışın koşularını Akışlar ekranında aç"
                  className="ml-auto flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                >
                  <GitBranch size={13} /> Akış görünümü
                </button>
              )}
            </header>
            {selected.running && (
              <div className="flex items-center gap-2 border-b border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-success)_8%,transparent)] px-6 py-2">
                <Loader2
                  size={14}
                  className="shrink-0 animate-spin text-[var(--color-success)]"
                />
                <span className="text-xs font-medium text-[var(--color-success)]">
                  Yanıt hazırlanıyor…
                </span>
                <span className="ml-auto flex gap-1">
                  {[0, 1, 2].map((i) => (
                    <span
                      key={i}
                      className="inline-block h-1.5 w-1.5 rounded-full bg-[var(--color-success)]"
                      style={{ animation: `pulse 1.2s ease-in-out ${i * 0.2}s infinite` }}
                    />
                  ))}
                </span>
              </div>
            )}
            <MessageList
              messages={messages}
              pending={loading}
              agents={agents}
              onOpenFile={onOpenFile}
              onOpenArtifact={onOpenArtifact}
            />
          </>
        ) : (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-[var(--color-text-dim)]">
            <Activity size={32} strokeWidth={1.5} />
            <p className="text-sm">Bir yürütme seç ve transkriptini görüntüle.</p>
          </div>
        )}
      </div>

      {spawnOpen && (
        <SpawnSessionModal
          agents={agents}
          onClose={() => setSpawnOpen(false)}
          onError={onError}
          onSpawned={(sessionId) => {
            setSpawnOpen(false)
            setFilter('spawned')
            select(sessionId)
            // Refresh the list so the new spawn appears immediately.
            setTimeout(() => load('spawned'), 100)
          }}
        />
      )}
    </div>
  )
}
