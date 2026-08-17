import { resolveAgent } from '@/shared/lib/agentLookup'
import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Settings,
  Pencil,
  Sparkles,
  ClipboardCopy,
  Trash2,
  Search,
  X,
  MessageSquareText,
  Plus,
  RefreshCw,
  Archive,
  ArchiveRestore,
  Pin,
  PinOff,
  Table2,
  Users,
  type LucideIcon,
} from 'lucide-react'
import type { Agent, Session, SearchHit } from '@/types'
import { api } from '@/api'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { relativeTime, bucketOf, BUCKET_LABELS, BUCKET_ORDER, type Bucket } from '@/shared/lib/time'
import { modelDisplayName } from '@/shared/lib/modelLabel'
import { useOutsideClick } from '@/shared/hooks/useOutsideClick'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { SelectionBar, SelectionBarButton, Skeleton } from '@/shared/components'
import { useDelayedFlag } from '@/shared/hooks/useDelayedFlag'
import {
  FILTERS,
  kindMeta,
  matchesKindFilter,
  RunStateBadge,
  StatusPill,
  type SessionListTab,
} from './sessionKindMeta'
import type { ExecutionRuntime } from '@/app/useExecutionRuntime'
import { isWorkerSession } from '@/shared/lib/coordination'

interface Props {
  sessions: Session[]
  agents: Agent[]
  activeSessionId: string | null
  // Sessions whose turns are currently being generated (live), shown with a
  // pulsing indicator so in-progress conversations are visible from the list.
  // A set because several turns can stream concurrently (detached server-side).
  streamingSessionIds?: ReadonlySet<string>
  // sessionId → live-running flag + last task/flow run status, from
  // GET /api/executions. Covers autonomous turns this window never streamed.
  runtimeById?: ReadonlyMap<string, ExecutionRuntime>
  // True while the workspace's session list is still being fetched — the rows are
  // replaced by skeletons so the column never claims "no sessions" prematurely.
  loading?: boolean
  newDisabled: boolean
  // TSK68 load-more: total/hasMore from the paged /api/sessions envelope and the
  // callback that appends the next page. When hasMore is true the list renders a
  // "Daha fazla yükle" row at the bottom.
  totalSessions?: number
  hasMoreSessions?: boolean
  onLoadMore?: () => void
  // Tab state is owned by the app so it can live in the URL (deep-linkable /
  // back-forward aware): ?list=active|archived|workers and ?kind=<filter key>.
  view: SessionListTab
  onViewChange: (v: SessionListTab) => void
  kindFilter: string
  onKindFilterChange: (k: string) => void
  // Open the bulk sessions table (searchable/sortable grid of every session).
  onOpenOverview: () => void
  // messageId is set when the user clicks a message-content search result, so the
  // transcript can scroll to that exact turn.
  onSelectSession: (id: string, messageId?: string) => void
  onNewSession: () => void
  // Re-fetch the session list from the server (manual refresh button).
  onRefresh: () => void
  onRenameSession: (id: string, title: string) => void
  onGenerateTitle: (id: string) => void
  onCopyPath: (id: string) => void
  onDeleteSession: (id: string) => void
  // Archive (true) or restore (false) a session — drives the Active/Archived filter.
  onSetArchived: (id: string, archived: boolean) => void
  // Pin (true) or unpin (false) a session — pinned rows float to the top.
  onSetPinned: (id: string, pinned: boolean) => void
}

// SessionsSidebar is the unified sessions column: a flat, time-bucketed list of
// every session (chat / task / flow / schedule / spawn, newest first), with a
// per-kind badge + filter tabs, live-running pulse, run status pill, unread dots
// and a settings menu (rename, AI title, copy path, open folder, delete).
export function SessionsSidebar({
  sessions,
  agents,
  activeSessionId,
  streamingSessionIds,
  runtimeById,
  loading = false,
  newDisabled,
  totalSessions,
  hasMoreSessions,
  onLoadMore,
  view,
  onViewChange,
  kindFilter,
  onKindFilterChange,
  onOpenOverview,
  onSelectSession,
  onNewSession,
  onRefresh,
  onRenameSession,
  onGenerateTitle,
  onCopyPath,
  onDeleteSession,
  onSetArchived,
  onSetPinned,
}: Props) {
  const [menuId, setMenuId] = useState<string | null>(null)
  // Top-level view: Active (default), Archived, or Workers (coordinator-spawned
  // worker sessions). Archiving moves a session into the Archived view — it is
  // never deleted. Worker sessions live in their own view so they don't clutter
  // the active list. Derived booleans keep the downstream filter logic terse.
  // `view` + `kindFilter` are props (URL-owned), see Props.
  const showArchived = view === 'archived'
  const showWorkers = view === 'workers'
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [renameText, setRenameText] = useState('')
  const [query, setQuery] = useState('')
  // Cross-session message-content search (CG-16). The same box filters session
  // titles locally AND, when the query is long enough, full-text searches every
  // session's messages via the backend (debounced).
  const [hits, setHits] = useState<SearchHit[]>([])
  const [searching, setSearching] = useState(false)
  // Close the open row menu on any outside click (detached while no menu is open).
  const rootRef = useOutsideClick<HTMLDivElement>(() => setMenuId(null), !!menuId)

  // Draggable width (persisted), matching the old sidebar behaviour.
  const MIN = 200
  const MAX = 560
  const [width, setWidth] = useState(() => {
    const saved = Number(localStorage.getItem('tionswarm.sidebarWidth'))
    return saved >= MIN && saved <= MAX ? saved : 264
  })
  const drag = useRef<{ startX: number; startW: number } | null>(null)
  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!drag.current) return
      setWidth(
        Math.min(MAX, Math.max(MIN, drag.current.startW + (e.clientX - drag.current.startX))),
      )
    }
    const onUp = () => {
      if (!drag.current) return
      drag.current = null
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
      localStorage.setItem('tionswarm.sidebarWidth', String(width))
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
  }, [width])
  const startDrag = (e: React.MouseEvent) => {
    e.preventDefault()
    drag.current = { startX: e.clientX, startW: width }
    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'col-resize'
  }

  // Per-tab activity counts: for each view's scope, how many sessions are
  // ongoing (a turn is streaming or an autonomous run is live) vs completed
  // (idle/finished). Surfaced on every tab so live work is visible without
  // opening it. `isLive` mirrors the per-row streaming check below.
  // The Workers scope uses the lineage helper, not `role === 'worker'`: a
  // mid-level node of a nested coordinator tree is a worker too, and counting
  // only leaves would hide whole branches.
  const tabStats = useMemo(() => {
    const isLive = (s: Session) =>
      (streamingSessionIds?.has(s.id) ?? false) || (runtimeById?.get(s.id)?.running ?? false)
    const stat = (inScope: (s: Session) => boolean) => {
      let ongoing = 0
      let total = 0
      for (const s of sessions) {
        if (!inScope(s)) continue
        total++
        if (isLive(s)) ongoing++
      }
      return { total, ongoing, completed: total - ongoing }
    }
    return {
      active: stat((s) => s.state !== 'archived' && !isWorkerSession(s)),
      workers: stat((s) => isWorkerSession(s) && s.state !== 'archived'),
      archived: stat((s) => s.state === 'archived'),
    }
  }, [sessions, streamingSessionIds, runtimeById])

  // Group the (already newest-first) sessions into recency buckets, preserving
  // order. The view (Active/Archived/Workers) narrows first, then the kind tab,
  // then a title search. Active EXCLUDES workers (they have their own tab) so the
  // default list isn't buried under coordinator-spawned sessions.
  const groups = useMemo(() => {
    const q = query.trim().toLowerCase()
    const map = new Map<Bucket, Session[]>()
    for (const s of sessions) {
      const isArchived = s.state === 'archived'
      const isWorker = isWorkerSession(s)
      if (showWorkers) {
        if (!isWorker || isArchived) continue
      } else if (showArchived) {
        if (!isArchived) continue
      } else {
        // Active view: hide archived AND worker sessions.
        if (isArchived || isWorker) continue
      }
      if (!matchesKindFilter(s.kind, kindFilter)) continue
      if (q && !(s.title || 'Yeni sohbet').toLowerCase().includes(q)) continue
      const b = bucketOf(s.updatedAt)
      const arr = map.get(b) ?? []
      arr.push(s)
      map.set(b, arr)
    }
    return BUCKET_ORDER.filter((b) => map.has(b)).map((b) => ({ bucket: b, items: map.get(b)! }))
  }, [sessions, query, showArchived, showWorkers, kindFilter])

  // Multi-select (Ctrl/Cmd+Click, Shift-range). The ordered id list is the
  // flattened visible render order so Shift+Click can span recency buckets.
  const sel = useMultiSelect()
  const orderedIds = useMemo(() => groups.flatMap((g) => g.items.map((s) => s.id)), [groups])
  // Selected sessions that are currently filtered out of view — bulk actions
  // still apply to them, so we surface the count.
  const hiddenSelected = useMemo(
    () => [...sel.selected].filter((id) => !orderedIds.includes(id)).length,
    [sel.selected, orderedIds],
  )
  const selectedIds = () => [...sel.selected]
  // Bulk actions reuse the existing per-id handlers in a loop (no new API).
  const bulkArchive = (archived: boolean) => {
    selectedIds().forEach((id) => onSetArchived(id, archived))
    sel.clear()
  }
  const bulkPin = (pinned: boolean) => {
    selectedIds().forEach((id) => onSetPinned(id, pinned))
    sel.clear()
  }
  const bulkTitle = () => {
    selectedIds().forEach((id) => {
      const s = sessions.find((x) => x.id === id)
      if (s && s.messageCount > 0) onGenerateTitle(id)
    })
    sel.clear()
  }
  const bulkDelete = () => {
    const ids = selectedIds()
    if (ids.length === 0) return
    if (confirm(`${ids.length} oturum silinsin mi? Bu işlem geri alınamaz.`)) {
      ids.forEach((id) => onDeleteSession(id))
      sel.clear()
    }
  }

  // Debounced full-text message search. Runs only for queries of 2+ chars so a
  // single keystroke doesn't hit the backend; cleared when the box empties.
  useEffect(() => {
    const q = query.trim()
    if (q.length < 2) {
      setHits([])
      setSearching(false)
      return
    }
    setSearching(true)
    let cancelled = false
    const handle = setTimeout(() => {
      api
        .searchMessages(q, { limit: 20 })
        .then((r) => {
          if (!cancelled) setHits(r)
        })
        .catch(() => {
          if (!cancelled) setHits([])
        })
        .finally(() => {
          if (!cancelled) setSearching(false)
        })
    }, 250)
    return () => {
      cancelled = true
      clearTimeout(handle)
    }
  }, [query])

  const startRename = (s: Session) => {
    setRenamingId(s.id)
    setRenameText(s.title || '')
    setMenuId(null)
  }
  const commitRename = (id: string) => {
    const t = renameText.trim()
    if (t) onRenameSession(id, t)
    setRenamingId(null)
  }

  // Delayed so a sub-100ms local load never flashes placeholder rows. While
  // `loading` holds but the delay has not elapsed, the list body renders nothing
  // — the "no sessions" copy stays suppressed either way.
  const showSkeleton = useDelayedFlag(loading)

  return (
    <aside
      ref={rootRef}
      style={{ width }}
      className="relative flex h-full shrink-0 flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)]"
    >
      <div className="flex items-center justify-between px-4 pt-4 pb-1">
        <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
          Oturumlar
        </span>
        <button
          onClick={onRefresh}
          className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
          title="Sohbet geçmişini yenile"
        >
          <RefreshCw size={14} />
        </button>
      </div>

      {/* Prominent new-chat button, above the search. */}
      <div className="px-3 pb-1 pt-1">
        <button
          onClick={onNewSession}
          disabled={newDisabled}
          className="flex w-full items-center justify-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:cursor-not-allowed disabled:opacity-40"
          title="Yeni oturum (varsayılan ajanla)"
        >
          <Plus size={15} /> Yeni Sohbet
        </button>
      </div>

      {/* Bulk sessions table (searchable/sortable grid of every session). */}
      <div className="px-3 pb-1">
        <button
          onClick={onOpenOverview}
          title="Tüm oturumları tablo olarak gör"
          data-testid="sessions-overview-open"
          className="flex w-full items-center justify-center gap-2 rounded-lg border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
        >
          <Table2 size={14} /> Oturumlar
        </button>
      </div>

      {/* Title search */}
      <div className="relative px-3 pb-2 pt-1">
        <Search
          size={13}
          className="pointer-events-none absolute left-5 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]"
        />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Oturum + mesaj ara…"
          className="w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] py-1.5 pl-7 pr-7 text-xs outline-none focus:border-[var(--color-accent)]"
        />
        {query && (
          <button
            onClick={() => setQuery('')}
            title="Temizle"
            className="absolute right-4 top-1/2 -translate-y-1/2 rounded p-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
          >
            <X size={13} />
          </button>
        )}
      </div>

      {/* Active / Workers / Archived filter. Each tab surfaces its live-vs-finished
          counts (green pulse = ongoing, muted = completed) so activity is visible
          without opening it. Archived is last (least-used). */}
      <div className="flex gap-1 px-3 pb-2">
        <button
          onClick={() => onViewChange('active')}
          className={`flex flex-1 items-center justify-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition ${
            view === 'active'
              ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
              : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
          }`}
        >
          Aktif
          <TabActivity ongoing={tabStats.active.ongoing} completed={tabStats.active.completed} />
        </button>
        <button
          onClick={() => onViewChange('workers')}
          title="Koordinatör tarafından başlatılan worker oturumları"
          className={`flex flex-1 items-center justify-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition ${
            view === 'workers'
              ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
              : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
          }`}
        >
          <Users size={12} /> Workers
          <TabActivity ongoing={tabStats.workers.ongoing} completed={tabStats.workers.completed} />
        </button>
        <button
          onClick={() => onViewChange('archived')}
          className={`flex flex-1 items-center justify-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition ${
            view === 'archived'
              ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
              : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
          }`}
        >
          <Archive size={12} /> Arşiv
          <TabActivity
            ongoing={tabStats.archived.ongoing}
            completed={tabStats.archived.completed}
          />
        </button>
      </div>

      {/* Kind filter tabs: the sidebar lists every session kind, so this is the
          antidote to a crowded list. Defaults to "Tümü" and is persisted. */}
      <div className="flex flex-wrap gap-1 px-3 pb-2" data-testid="session-kind-filters">
        {FILTERS.map((f) => (
          <button
            key={f.key}
            onClick={() => onKindFilterChange(f.key)}
            className={`rounded-full px-2.5 py-1 text-[11px] transition ${
              kindFilter === f.key
                ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto px-2 pb-2">
        {loading && (
          <div data-testid="sessions-skeleton" className="flex flex-col gap-1 px-1 pt-3">
            {showSkeleton &&
              Array.from({ length: 5 }, (_, i) => <Skeleton key={i} className="h-10 w-full" />)}
          </div>
        )}
        {!loading &&
          groups.map(({ bucket, items }) => (
            <div key={bucket} className="mb-1">
              <div className="px-3 pt-3 pb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
                {BUCKET_LABELS[bucket]}
              </div>
              {items.map((s) => {
                // Sessions outlive their agent, so resolve rather than find:
                // a deleted owner still renders (badged) instead of vanishing.
                const owner = resolveAgent(agents, s.agentId)
                const isActive = activeSessionId === s.id
                const runtime = runtimeById?.get(s.id)
                // Live either because THIS window is streaming the turn (instant, no
                // poll lag) or because the executions poll reports it running (covers
                // autonomous task/flow/schedule turns this window never streamed).
                const isStreaming =
                  (streamingSessionIds?.has(s.id) ?? false) || (runtime?.running ?? false)
                const meta = kindMeta(s.kind)
                const KindIcon = meta.icon
                const isSelected = sel.isSelected(s.id)
                return (
                  <div
                    key={s.id}
                    className={`group relative mb-0.5 flex w-full items-center rounded-lg pr-1 text-sm transition ${
                      isSelected
                        ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)] ring-1 ring-[var(--color-accent)]'
                        : isActive
                          ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
                          : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
                    }`}
                  >
                    {renamingId === s.id ? (
                      <input
                        autoFocus
                        value={renameText}
                        onChange={(e) => setRenameText(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') commitRename(s.id)
                          if (e.key === 'Escape') setRenamingId(null)
                        }}
                        onBlur={() => commitRename(s.id)}
                        className="m-1 flex-1 rounded border border-[var(--color-accent)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
                      />
                    ) : (
                      <button
                        onClick={(e) => {
                          // Ctrl/Cmd or Shift turns the click into a selection
                          // gesture; a plain click opens the session as before.
                          if (sel.handleClick(e, s.id, orderedIds, activeSessionId)) return
                          onSelectSession(s.id)
                        }}
                        className="flex min-w-0 flex-1 items-center gap-2 px-3 py-2 text-left"
                      >
                        {owner ? (
                          <AgentAvatar agent={owner} size={20} />
                        ) : (
                          <span className="h-5 w-5 shrink-0" />
                        )}
                        <span className="flex min-w-0 flex-1 flex-col">
                          <span className="flex items-center gap-1.5">
                            {isStreaming ? (
                              // Live turn in progress: a pulsing dot takes precedence
                              // over the unread dot.
                              <span
                                className="relative flex h-2 w-2 shrink-0"
                                title="Yanıt üretiliyor"
                              >
                                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--color-success)] opacity-75" />
                                <span className="relative inline-flex h-2 w-2 rounded-full bg-[var(--color-success)]" />
                              </span>
                            ) : (
                              s.unread && (
                                <span
                                  className="h-2 w-2 shrink-0 rounded-full bg-[var(--color-accent)]"
                                  title="Okunmadı"
                                />
                              )
                            )}
                            {s.pinned && (
                              <Pin
                                size={11}
                                className="shrink-0 -rotate-45 text-[var(--color-accent)]"
                              />
                            )}
                            <span
                              className={`min-w-0 flex-1 truncate ${s.unread || isStreaming ? 'font-semibold text-[var(--color-text)]' : ''}`}
                            >
                              {s.title || 'Yeni sohbet'}
                            </span>
                            {s.kind === 'spawned' && (
                              <span
                                className="shrink-0 rounded-full bg-[var(--color-surface-2)] px-1.5 py-px text-[9px] text-[var(--color-text-dim)]"
                                title={
                                  s.parentSessionId
                                    ? 'Bir devralma (handoff) ile oluşturuldu'
                                    : 'Spawn ile oluşturuldu'
                                }
                              >
                                {s.parentSessionId ? '↩ handoff' : '✦ spawn'}
                              </span>
                            )}
                            {/* Finished task/flow runs carry a pass/fail pill. */}
                            {!isStreaming && runtime?.lastStatus && (
                              <StatusPill status={runtime.lastStatus} />
                            )}
                            {/* How the last BACKGROUND turn ended (worker/spawn/
                              schedule runs). Independent of the archive filter:
                              this is the run outcome, `state` is visibility.
                              Hidden while a turn is live — the pulsing dot and
                              "yazıyor…" already say what is happening now, and a
                              stale outcome next to them reads as contradictory. */}
                            {!isStreaming && <RunStateBadge runState={s.runState} />}
                          </span>
                          {/* Meta row: kind badge + status/time on the left, the
                            session ID on a row of its own below the title. */}
                          <span className="flex items-center gap-1.5 text-[10px]">
                            <KindIcon size={11} className="shrink-0 opacity-60" />
                            <span className="shrink-0 opacity-60">{meta.label}</span>
                            {s.model && (
                              <span
                                className="shrink-0 rounded bg-[var(--color-surface-2)] px-1 py-px font-mono text-[9px] text-[var(--color-text-dim)]"
                                title={`Model: ${s.model}`}
                              >
                                {modelDisplayName(s.model)}
                              </span>
                            )}
                            {isStreaming ? (
                              <span className="truncate font-medium text-[var(--color-success)]">
                                yazıyor…
                              </span>
                            ) : (
                              <span className="truncate opacity-60">
                                · {relativeTime(s.updatedAt)} · {s.messageCount} mesaj
                              </span>
                            )}
                            <span
                              className="ml-auto shrink-0 font-mono opacity-50"
                              title="Oturum ID"
                            >
                              {s.id}
                            </span>
                          </span>
                        </span>
                      </button>
                    )}

                    <button
                      onClick={() => setMenuId((v) => (v === s.id ? null : s.id))}
                      title="Oturum ayarları"
                      className="ml-1 shrink-0 rounded p-1 text-[var(--color-text-dim)] opacity-0 transition hover:text-[var(--color-accent)] group-hover:opacity-100"
                    >
                      <Settings size={16} />
                    </button>

                    {menuId === s.id && (
                      <div className="absolute right-1 top-9 z-20 w-44 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 text-sm shadow-xl">
                        <MenuItem
                          icon={Pencil}
                          label="Başlığı düzenle"
                          onClick={() => startRename(s)}
                        />
                        <MenuItem
                          icon={Sparkles}
                          label="AI ile başlık"
                          disabled={s.messageCount === 0}
                          onClick={() => {
                            onGenerateTitle(s.id)
                            setMenuId(null)
                          }}
                        />
                        <MenuItem
                          icon={ClipboardCopy}
                          label="Yolu kopyala"
                          onClick={() => {
                            onCopyPath(s.id)
                            setMenuId(null)
                          }}
                        />
                        <MenuItem
                          icon={s.pinned ? PinOff : Pin}
                          label={s.pinned ? 'Sabitlemeyi kaldır' : 'Üste sabitle'}
                          onClick={() => {
                            onSetPinned(s.id, !s.pinned)
                            setMenuId(null)
                          }}
                        />
                        {s.state === 'archived' ? (
                          <MenuItem
                            icon={ArchiveRestore}
                            label="Arşivden çıkar"
                            onClick={() => {
                              onSetArchived(s.id, false)
                              setMenuId(null)
                            }}
                          />
                        ) : (
                          <MenuItem
                            icon={Archive}
                            label="Arşivle"
                            onClick={() => {
                              onSetArchived(s.id, true)
                              setMenuId(null)
                            }}
                          />
                        )}
                        <div className="my-1 border-t border-[var(--color-border)]" />
                        <MenuItem
                          icon={Trash2}
                          label="Sil"
                          danger
                          onClick={() => {
                            setMenuId(null)
                            if (confirm(`"${s.title || 'Bu oturum'}" silinsin mi?`))
                              onDeleteSession(s.id)
                          }}
                        />
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          ))}
        {!loading && groups.length === 0 && query.trim().length < 2 && (
          <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
            {showWorkers
              ? 'Worker oturumu yok. Koordinatör modunda spawn_worker ile başlatın.'
              : showArchived
                ? 'Arşivlenmiş oturum yok.'
                : kindFilter
                  ? 'Bu türde oturum yok.'
                  : 'Oturum yok. + ile başlat.'}
          </p>
        )}

        {/* TSK68 load-more: the list is paged; append the next page instead of
            fetching every session up front. Hidden while searching (message hits
            are their own section) or when the full list is already loaded. */}
        {!loading &&
          hasMoreSessions &&
          onLoadMore &&
          query.trim().length < 2 &&
          groups.length > 0 && (
            <button
              onClick={onLoadMore}
              data-testid="sessions-load-more"
              className="mx-1 mt-2 flex w-[calc(100%-0.5rem)] items-center justify-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs font-medium text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
            >
              Daha fazla yükle
              {totalSessions !== undefined && (
                <span className="opacity-60">
                  ({sessions.length}/{totalSessions})
                </span>
              )}
            </button>
          )}

        {/* Cross-session message matches (CG-16) — shown whenever a search is active. */}
        {query.trim().length >= 2 && (
          <div className="mt-2 border-t border-[var(--color-border)] pt-2">
            <div className="flex items-center gap-1.5 px-3 pb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
              <MessageSquareText size={11} />
              Mesajlarda{searching ? '…' : hits.length ? ` (${hits.length})` : ''}
            </div>
            {!searching && hits.length === 0 && (
              <p className="px-3 py-1.5 text-xs text-[var(--color-text-dim)]">Eşleşen mesaj yok.</p>
            )}
            {hits.map((h) => (
              <button
                key={h.messageId}
                onClick={() => onSelectSession(h.sessionId, h.messageId)}
                className="mb-0.5 flex w-full flex-col gap-0.5 rounded-lg px-3 py-2 text-left transition hover:bg-[var(--color-surface-2)]"
              >
                <span className="flex items-center gap-1.5 text-[11px] text-[var(--color-text-dim)]">
                  <span className="rounded bg-[var(--color-surface-2)] px-1 py-px text-[9px] uppercase tracking-wide">
                    {h.role === 'assistant' ? 'ajan' : h.role === 'user' ? 'kullanıcı' : h.role}
                  </span>
                  <span className="min-w-0 flex-1 truncate font-medium text-[var(--color-text)]">
                    {h.sessionTitle || 'Yeni sohbet'}
                  </span>
                  <span className="shrink-0">{relativeTime(h.createdAt)}</span>
                </span>
                <span className="line-clamp-2 text-xs text-[var(--color-text-dim)]">
                  {h.snippet}
                </span>
              </button>
            ))}
          </div>
        )}
      </div>

      <SelectionBar
        count={sel.count}
        hiddenCount={hiddenSelected}
        onClear={sel.clear}
        onSelectAll={orderedIds.length ? () => sel.selectAll(orderedIds) : undefined}
      >
        <SelectionBarButton icon={<Sparkles size={13} />} onClick={bulkTitle}>
          AI başlık
        </SelectionBarButton>
        <SelectionBarButton icon={<Pin size={13} />} onClick={() => bulkPin(true)}>
          Sabitle
        </SelectionBarButton>
        {showArchived ? (
          <SelectionBarButton
            icon={<ArchiveRestore size={13} />}
            onClick={() => bulkArchive(false)}
          >
            Arşivden çıkar
          </SelectionBarButton>
        ) : (
          <SelectionBarButton icon={<Archive size={13} />} onClick={() => bulkArchive(true)}>
            Arşivle
          </SelectionBarButton>
        )}
        <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
          Sil
        </SelectionBarButton>
      </SelectionBar>

      <div
        onMouseDown={startDrag}
        title="Genişliği ayarla"
        className="absolute right-0 top-0 h-full w-1 cursor-col-resize bg-transparent transition hover:bg-[var(--color-accent)]"
      />
    </aside>
  )
}

// Compact per-tab activity indicator: a green pulsing dot + count for ongoing
// (live) sessions, and a muted count for completed (idle/finished) ones. Each
// half hides when zero, so an empty tab shows nothing.
function TabActivity({ ongoing, completed }: { ongoing: number; completed: number }) {
  if (ongoing === 0 && completed === 0) return null
  return (
    <span className="flex items-center gap-1 text-[10px]">
      {ongoing > 0 && (
        <span
          className="flex items-center gap-0.5 font-semibold text-[var(--color-success)]"
          title={`${ongoing} devam eden`}
        >
          <span className="relative flex h-1.5 w-1.5">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--color-success)] opacity-75" />
            <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-[var(--color-success)]" />
          </span>
          {ongoing}
        </span>
      )}
      {completed > 0 && (
        <span className="opacity-60" title={`${completed} tamamlanan`}>
          {completed}
        </span>
      )}
    </span>
  )
}

function MenuItem({
  icon: Icon,
  label,
  onClick,
  disabled,
  danger,
}: {
  icon: LucideIcon
  label: string
  onClick: () => void
  disabled?: boolean
  danger?: boolean
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`flex w-full items-center gap-2 rounded px-2.5 py-1.5 text-left transition disabled:opacity-30 ${
        danger
          ? 'text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]'
          : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
      }`}
    >
      <Icon size={14} className="shrink-0" />
      {label}
    </button>
  )
}
