import { useMemo, useState } from 'react'
import { Search, X, ArrowUp, ArrowDown, Copy, type LucideIcon } from 'lucide-react'
import type { Agent, Execution } from '@/types'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { relativeTime, fullDateTime, formatDuration } from '@/shared/lib/time'
import { ModalOverlay } from '@/shared/components'
import { FILTERS, kindMeta, shortId, StatusPill } from './executionsShared'

interface Props {
  // All executions currently loaded by the panel (workspace-wide session rows).
  items: Execution[]
  agents: Agent[]
  // Select an execution (jump to its transcript) — the overlay closes afterwards.
  onSelect: (sessionId: string) => void
  onClose: () => void
}

// Sortable columns. `get` maps a row to a comparable value; string values sort
// case-insensitively, numbers numerically.
type SortKey = 'title' | 'kind' | 'agent' | 'messages' | 'status' | 'duration' | 'created' | 'updated'

// runElapsed is the wall-clock span of a run in seconds: for a finished run the
// created→updated gap; for a live one, created→now (so the table keeps ticking).
function runElapsed(it: Execution, nowSec: number): number {
  const end = it.running ? nowSec : it.updatedAt
  return Math.max(0, end - it.createdAt)
}

// statusRank orders rows by a coarse lifecycle priority for the status column
// sort: running first, then failures, then successes, then the rest.
function statusRank(it: Execution): number {
  if (it.running) return 0
  if (it.lastStatus === 'failure') return 1
  if (it.lastStatus === 'success') return 2
  return 3
}

const COLUMNS: { key: SortKey; label: string; align?: 'right' }[] = [
  { key: 'title', label: 'Başlık' },
  { key: 'kind', label: 'Tür' },
  { key: 'agent', label: 'Ajan' },
  { key: 'messages', label: 'Mesaj', align: 'right' },
  { key: 'status', label: 'Durum' },
  { key: 'duration', label: 'Süre', align: 'right' },
  { key: 'created', label: 'Oluşturma', align: 'right' },
  { key: 'updated', label: 'Güncelleme', align: 'right' },
]

// SessionsOverview is the bulk sessions table: a searchable, filterable, sortable
// grid of every execution (chat / task / flow / schedule / spawn) in the
// workspace. It reuses the panel's already-loaded executions list — no extra
// fetch — and clicking a row jumps to that transcript in the panel.
export function SessionsOverview({ items, agents, onSelect, onClose }: Props) {
  const [query, setQuery] = useState('')
  const [kind, setKind] = useState('')
  const [sortKey, setSortKey] = useState<SortKey>('updated')
  const [asc, setAsc] = useState(false)
  // Snapshot "now" once per render so live-duration values are stable within a
  // sort pass (avoids rows shuffling under a Date.now() called per-comparison).
  const nowSec = Math.floor(Date.now() / 1000)

  const agentById = useMemo(() => new Map(agents.map((a) => [a.id, a])), [agents])

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    const filtered = items.filter((it) => {
      if (kind && it.kind !== kind) return false
      if (!q) return true
      return (
        (it.title || '').toLowerCase().includes(q) ||
        it.sessionId.toLowerCase().includes(q) ||
        (it.agentName || '').toLowerCase().includes(q)
      )
    })
    const val = (it: Execution): string | number => {
      switch (sortKey) {
        case 'title':
          return (it.title || kindMeta(it.kind).label).toLowerCase()
        case 'kind':
          return kindMeta(it.kind).label.toLowerCase()
        case 'agent':
          return (it.agentName || '').toLowerCase()
        case 'messages':
          return it.messageCount
        case 'status':
          return statusRank(it)
        case 'duration':
          return runElapsed(it, nowSec)
        case 'created':
          return it.createdAt
        case 'updated':
          return it.updatedAt
      }
    }
    const dir = asc ? 1 : -1
    return [...filtered].sort((a, b) => {
      const va = val(a)
      const vb = val(b)
      if (va < vb) return -1 * dir
      if (va > vb) return 1 * dir
      return 0
    })
  }, [items, query, kind, sortKey, asc, nowSec])

  // Toggle sort: same column flips direction, a new column selects it (numeric
  // and timestamp columns default to descending — biggest/newest first).
  const sortBy = (key: SortKey) => {
    if (key === sortKey) {
      setAsc((v) => !v)
    } else {
      setSortKey(key)
      setAsc(key === 'title' || key === 'kind' || key === 'agent')
    }
  }

  const SortIcon = asc ? ArrowUp : ArrowDown

  return (
    <ModalOverlay onClose={onClose}>
      <div
        className="flex max-h-[86vh] w-[min(1100px,94vw)] flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl"
        data-testid="sessions-overview"
      >
        {/* Header: title + result count + close. */}
        <div className="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-3">
          <div className="flex items-baseline gap-2">
            <h2 className="text-sm font-semibold text-[var(--color-text)]">Oturumlar</h2>
            <span className="text-xs text-[var(--color-text-dim)]">{rows.length} / {items.length}</span>
          </div>
          <button
            onClick={onClose}
            title="Kapat"
            className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
          >
            <X size={16} />
          </button>
        </div>

        {/* Toolbar: search box + kind filter tabs. */}
        <div className="flex flex-wrap items-center gap-3 border-b border-[var(--color-border)] px-5 py-2.5">
          <div className="relative flex-1 min-w-[200px]">
            <Search
              size={14}
              className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]"
            />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Başlık, kimlik veya ajanda ara…"
              className="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] py-1.5 pl-8 pr-8 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-accent)]"
            />
            {query && (
              <button
                onClick={() => setQuery('')}
                title="Temizle"
                className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
              >
                <X size={13} />
              </button>
            )}
          </div>
          <div className="flex flex-wrap gap-1">
            {FILTERS.map((f) => (
              <button
                key={f.key}
                onClick={() => setKind(f.key)}
                className={`rounded-full px-2.5 py-1 text-[11px] transition ${
                  kind === f.key
                    ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                    : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
                }`}
              >
                {f.label}
              </button>
            ))}
          </div>
        </div>

        {/* Table. */}
        <div className="min-h-0 flex-1 overflow-auto">
          <table className="w-full border-collapse text-sm">
            <thead className="sticky top-0 z-10 bg-[var(--color-surface)]">
              <tr className="border-b border-[var(--color-border)]">
                {COLUMNS.map((c) => (
                  <th
                    key={c.key}
                    onClick={() => sortBy(c.key)}
                    className={`cursor-pointer select-none whitespace-nowrap px-3 py-2 text-[11px] font-medium uppercase tracking-wide text-[var(--color-text-dim)] transition hover:text-[var(--color-text)] ${
                      c.align === 'right' ? 'text-right' : 'text-left'
                    }`}
                  >
                    <span className={`inline-flex items-center gap-1 ${c.align === 'right' ? 'flex-row-reverse' : ''}`}>
                      {c.label}
                      {sortKey === c.key && <SortIcon size={11} />}
                    </span>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((it) => {
                const meta = kindMeta(it.kind)
                const Icon: LucideIcon = meta.icon
                const owner = agentById.get(it.agentId)
                return (
                  <tr
                    key={it.sessionId}
                    onClick={() => {
                      onSelect(it.sessionId)
                      onClose()
                    }}
                    className="cursor-pointer border-b border-[var(--color-border)] transition hover:bg-[var(--color-surface-2)]"
                  >
                    {/* Title + live/unread dot */}
                    <td className="max-w-[280px] px-3 py-2">
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
                          className={`truncate ${
                            it.unread || it.running
                              ? 'font-semibold text-[var(--color-text)]'
                              : 'text-[var(--color-text)]'
                          }`}
                        >
                          {it.title || meta.label}
                        </span>
                      </span>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          void copyToClipboard(it.sessionId)
                        }}
                        title={`Kimliği kopyala: ${it.sessionId}`}
                        className="mt-0.5 inline-flex items-center gap-1 font-mono text-[10px] text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                      >
                        <Copy size={9} /> #{shortId(it.sessionId)}
                      </button>
                    </td>
                    {/* Kind */}
                    <td className="whitespace-nowrap px-3 py-2">
                      <span className="inline-flex items-center gap-1.5 text-[var(--color-text-dim)]">
                        <Icon size={13} /> {meta.label}
                      </span>
                    </td>
                    {/* Agent */}
                    <td className="whitespace-nowrap px-3 py-2">
                      <span className="inline-flex items-center gap-1.5 text-[var(--color-text-dim)]">
                        {owner && <AgentAvatar agent={owner} size={16} />}
                        <span className="truncate">{it.agentName || '—'}</span>
                      </span>
                    </td>
                    {/* Messages */}
                    <td className="px-3 py-2 text-right tabular-nums text-[var(--color-text-dim)]">
                      {it.messageCount}
                    </td>
                    {/* Status */}
                    <td className="whitespace-nowrap px-3 py-2">
                      {it.running ? (
                        <span className="text-[11px] font-medium text-[var(--color-success)]">çalışıyor…</span>
                      ) : (
                        <StatusPill status={it.lastStatus ?? ''} />
                      )}
                    </td>
                    {/* Duration */}
                    <td className="whitespace-nowrap px-3 py-2 text-right tabular-nums text-[var(--color-text-dim)]">
                      {formatDuration(runElapsed(it, nowSec))}
                    </td>
                    {/* Created */}
                    <td
                      className="whitespace-nowrap px-3 py-2 text-right text-[var(--color-text-dim)]"
                      title={fullDateTime(it.createdAt)}
                    >
                      {relativeTime(it.createdAt)}
                    </td>
                    {/* Updated */}
                    <td
                      className="whitespace-nowrap px-3 py-2 text-right text-[var(--color-text-dim)]"
                      title={fullDateTime(it.updatedAt)}
                    >
                      {relativeTime(it.updatedAt)}
                    </td>
                  </tr>
                )
              })}
              {rows.length === 0 && (
                <tr>
                  <td colSpan={COLUMNS.length} className="px-3 py-8 text-center text-xs text-[var(--color-text-dim)]">
                    {items.length === 0
                      ? 'Henüz oturum yok.'
                      : 'Aramayla eşleşen oturum bulunamadı.'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </ModalOverlay>
  )
}
