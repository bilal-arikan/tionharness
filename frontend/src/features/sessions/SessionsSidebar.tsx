import { resolveAgent } from '@/shared/lib/agentLookup'
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
} from 'react'
import {
  Sparkles,
  Trash2,
  Search,
  X,
  MessageSquareText,
  Plus,
  RefreshCw,
  Archive,
  ArchiveRestore,
  Pin,
  Users,
  PencilLine,
} from 'lucide-react'
import type { Agent, Session, SearchHit } from '@/types'
import { api } from '@/api'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { relativeTime, bucketOf, bucketLabel, BUCKET_ORDER, type Bucket } from '@/shared/lib/time'
import { modelDisplayName } from '@/shared/lib/modelLabel'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { SelectionBar, SelectionBarButton, Skeleton } from '@/shared/components'
import { useDelayedFlag } from '@/shared/hooks/useDelayedFlag'
import { useDraftSessionIds } from '@/shared/hooks/useDraftSessionIds'
import {
  ARCHIVED_CHIP,
  kindMeta,
  type ChipClickMode,
  SESSION_CHIPS,
  sessionMatchesChips,
  sessionLiveScope,
  WORKER_CHIP,
  SUBAGENT_CHIP,
  sessionChipKey,
} from './sessionKindMeta'
import { RunStateBadge, StatusPill } from './sessionKindBadges'
import type { ExecutionRuntime } from '@/app/useExecutionRuntime'
import { isWorkerSession } from '@/shared/lib/coordination'
import { shouldShowSessionsLoadMore } from './sessionsLoadMore'
import { sessionMatchesQuery } from './sessionSearch'
import { countLiveDescendantWorkers } from './liveWorkerCounts'
import { sessionListModel } from './sessionListModel'

const SIDEBAR_WIDTH_KEY = 'tionharness.sidebarWidth'
const MIN_SIDEBAR_WIDTH = 200
const MAX_SIDEBAR_WIDTH = 560
const DEFAULT_SIDEBAR_WIDTH = 264

function readSidebarWidth(): number {
  try {
    const saved = Number(globalThis.localStorage.getItem(SIDEBAR_WIDTH_KEY))
    return saved >= MIN_SIDEBAR_WIDTH && saved <= MAX_SIDEBAR_WIDTH ? saved : DEFAULT_SIDEBAR_WIDTH
  } catch {
    return DEFAULT_SIDEBAR_WIDTH
  }
}

function writeSidebarWidth(width: number): void {
  try {
    globalThis.localStorage.setItem(SIDEBAR_WIDTH_KEY, String(width))
  } catch {
    // Persistence is best-effort when storage is blocked by browser policy.
  }
}

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
  // "Daha fazla yükle" row at the bottom. total counts the CHIP-FILTERED set:
  // the server applies the same chip predicate before paging, so the footer
  // count and the button both describe rows this list can actually show.
  totalSessions?: number
  hasMoreSessions?: boolean
  onLoadMore?: () => void
  // Chip selection, owned above the sidebar because the session list request
  // carries it (see useSessionChips).
  chipsOff: string[]
  chipSet: ReadonlySet<string>
  onClickChip: (key: string, mode: ChipClickMode) => void
  // chipKey → workspace-wide count from the list response. Used for every chip
  // the server can classify; the two live chips stay counted over loaded rows
  // because liveness is client state.
  chipCountsFromServer?: Record<string, number>
  // messageId is set when the user clicks a message-content search result, so the
  // transcript can scroll to that exact turn.
  onSelectSession: (id: string, messageId?: string) => void
  onNewSession: () => void
  // Re-fetch the session list from the server (manual refresh button).
  onRefresh: () => void
  onGenerateTitle: (id: string) => void
  onDeleteSession: (id: string) => void
  // Archive (true) or restore (false) a session — drives the Active/Archived filter.
  onSetArchived: (id: string, archived: boolean) => void
  // Pin (true) or unpin (false) a session — pinned rows float to the top.
  onSetPinned: (id: string, pinned: boolean) => void
}

// SessionsSidebar is the unified sessions column: a flat, time-bucketed list of
// every session (chat / task / flow / schedule / spawn, newest first), with a
// per-kind badge + filter tabs, live-running pulse, run status pill, unread dots
// and a multi-select bulk bar (AI title, pin, archive, delete). Per-row actions
// live in the session inspector (title block + quick actions), not in the list.
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
  chipsOff,
  chipSet,
  onClickChip,
  chipCountsFromServer,
  onSelectSession,
  onNewSession,
  onRefresh,
  onGenerateTitle,
  onDeleteSession,
  onSetArchived,
  onSetPinned,
}: Props) {
  // One flat list filtered by multi-select chips: the kind chips plus a Worker
  // and an Arşiv chip. The selection itself lives in useSessionChips (above this
  // component) because the session list REQUEST carries it — the server pages
  // the filtered set, so paging and the chips have to share one source of truth.
  //
  // Plain click toggles one chip; Ctrl/Cmd-click solos it (everything else off),
  // Shift-click inverts every other chip.
  const clickChip = (key: string, e: ReactMouseEvent) => {
    const mode: ChipClickMode = e.ctrlKey || e.metaKey ? 'solo' : e.shiftKey ? 'invert' : 'toggle'
    onClickChip(key, mode)
  }
  const [query, setQuery] = useState('')
  // Cross-session message-content search (CG-16). The same box filters session
  // titles locally AND, when the query is long enough, full-text searches every
  // session's messages via the backend (debounced).
  const [hits, setHits] = useState<SearchHit[]>([])
  const [searching, setSearching] = useState(false)

  // Draggable width (persisted), matching the old sidebar behaviour.
  const [width, setWidth] = useState(readSidebarWidth)
  const drag = useRef<{ startX: number; startW: number } | null>(null)
  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!drag.current) return
      setWidth(
        Math.min(
          MAX_SIDEBAR_WIDTH,
          Math.max(MIN_SIDEBAR_WIDTH, drag.current.startW + (e.clientX - drag.current.startX)),
        ),
      )
    }
    const onUp = () => {
      if (!drag.current) return
      drag.current = null
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
      writeSidebarWidth(width)
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

  // Sessions holding an unsent composer draft (localStorage, active workspace).
  const draftIds = useDraftSessionIds()

  // coordinatorSessionId → how many workers anywhere below it have a live turn.
  // The executions feed carries the complete workspace lineage, so a deep worker
  // remains visible at the root even when worker rows are filtered or not paged in.
  const liveWorkerCounts = useMemo(
    () => countLiveDescendantWorkers(sessions, runtimeById, streamingSessionIds),
    [sessions, streamingSessionIds, runtimeById],
  )

  // chipShapeOf builds the one shape both the chip counters and the visible-list
  // filter classify a session with, so a chip's badge count can never disagree
  // with how many rows that chip actually keeps.
  const chipShapeOf = useCallback(
    (s: Session) => ({
      kind: s.kind,
      category: s.category,
      executionType: s.executionType,
      isWorker: isWorkerSession(s),
      isArchived: s.state === 'archived',
      isRunning:
        (streamingSessionIds?.has(s.id) ?? false) || (runtimeById?.get(s.id)?.running ?? false),
      liveWorkerCount: liveWorkerCounts.get(s.id) ?? 0,
    }),
    [streamingSessionIds, runtimeById, liveWorkerCounts],
  )

  // Chip badges. The server counts every chip it can classify over the WHOLE
  // workspace (before the chips filter), which is the only source that still
  // knows what an unticked chip hides now that its rows never reach this list.
  // The two live chips are client state, so they stay counted over loaded rows.
  const chipCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const [key, n] of Object.entries(chipCountsFromServer ?? {})) counts.set(key, n)
    for (const s of sessions) {
      const live = sessionLiveScope(chipShapeOf(s))
      if (live) counts.set(live, (counts.get(live) ?? 0) + 1)
    }
    return counts
  }, [sessions, chipShapeOf, chipCountsFromServer])

  // Group the (already newest-first) sessions into recency buckets, preserving
  // order. The chip selection narrows first, then a title/session ID search. The worker
  // check uses the lineage helper, not `role === 'worker'`: a mid-level node of
  // a nested coordinator tree is a worker too.
  const groups = useMemo(() => {
    const map = new Map<Bucket, Session[]>()
    for (const s of sessions) {
      if (!sessionMatchesChips(chipShapeOf(s), chipSet)) continue
      if (!sessionMatchesQuery(s, query)) continue
      // Pinned rows leave the recency buckets entirely and form their own group,
      // which BUCKET_ORDER renders first — otherwise an old pinned chat would sink
      // into "Daha eski" even though the list itself floats it to the top.
      const b: Bucket = s.pinned ? 'pinned' : bucketOf(s.updatedAt)
      const arr = map.get(b) ?? []
      arr.push(s)
      map.set(b, arr)
    }
    return BUCKET_ORDER.filter((b) => map.has(b)).map((b) => ({ bucket: b, items: map.get(b)! }))
  }, [sessions, query, chipSet, chipShapeOf])

  // Multi-select (Ctrl/Cmd+Click, Shift-range). The ordered id list is the
  // flattened visible render order so Shift+Click can span recency buckets.
  const sel = useMultiSelect()
  const orderedIds = useMemo(() => groups.flatMap((g) => g.items.map((s) => s.id)), [groups])
  // Membership lookups below run once per selected id; over an array both would
  // be O(selected × sessions), which is the whole list on every render.
  const orderedIdSet = useMemo(() => new Set(orderedIds), [orderedIds])
  const sessionById = useMemo(() => new Map(sessions.map((s) => [s.id, s])), [sessions])
  // Selected sessions that are currently filtered out of view — bulk actions
  // still apply to them, so we surface the count.
  const hiddenSelected = useMemo(
    () => [...sel.selected].filter((id) => !orderedIdSet.has(id)).length,
    [sel.selected, orderedIdSet],
  )
  const selectedIds = () => [...sel.selected]
  // With one flat list a selection can mix archived and live rows; the bulk
  // button only flips to "restore" when every selected session is archived.
  const allSelectedArchived = useMemo(
    () =>
      sel.count > 0 && [...sel.selected].every((id) => sessionById.get(id)?.state === 'archived'),
    [sel.count, sel.selected, sessionById],
  )
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

  // Delayed so a sub-100ms local load never flashes placeholder rows. While
  // `loading` holds but the delay has not elapsed, the list body renders nothing
  // — the "no sessions" copy stays suppressed either way.
  const showSkeleton = useDelayedFlag(loading)

  return (
    <aside
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

      {/* Local title/session ID filter plus debounced message search. */}
      <div className="relative px-3 pb-2 pt-1">
        <Search
          size={13}
          className="pointer-events-none absolute left-5 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]"
        />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Oturum adı, ID veya mesaj ara…"
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

      {/* Multi-select chips: every session kind plus the Worker and Arşiv scopes.
          All start selected — unticking a chip hides that slice. */}
      <div className="px-3 pb-1 text-[10px] text-[var(--color-text-dim)] opacity-70">Filtreler</div>
      <div className="flex flex-wrap gap-1 px-3 pb-2" data-testid="session-kind-filters">
        {SESSION_CHIPS.map((f) => {
          const on = chipSet.has(f.key)
          const Icon =
            f.key === WORKER_CHIP
              ? Users
              : f.key === ARCHIVED_CHIP
                ? Archive
                : f.key === SUBAGENT_CHIP
                  ? Sparkles
                  : null
          return (
            <button
              key={f.key}
              onClick={(e) => clickChip(f.key, e)}
              title={`${f.label}: ${chipCounts.get(f.key) ?? 0} oturum — Ctrl: yalnız bunu seç, Shift: diğerlerini tersle`}
              aria-pressed={on}
              data-chip={f.key}
              className={`flex items-center gap-1 rounded-full px-2.5 py-1 text-[11px] transition ${
                on
                  ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                  : 'text-[var(--color-text-dim)] opacity-60 hover:bg-[var(--color-surface-2)] hover:opacity-100'
              }`}
            >
              {Icon && <Icon size={11} />}
              {f.label}
              <span className="opacity-60">{chipCounts.get(f.key) ?? 0}</span>
            </button>
          )
        })}
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
                {bucketLabel(bucket)}
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
                // Its own turn is idle, but workers below it are running.
                const liveWorkers = liveWorkerCounts.get(s.id) ?? 0
                const hasDraft = draftIds.has(s.id)
                const meta = kindMeta(sessionChipKey(s))
                const KindIcon = meta.icon
                const isSelected = sel.isSelected(s.id)
                const displayedModel = sessionListModel(s.model, owner?.model, isStreaming)
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
                          ) : liveWorkers > 0 ? (
                            // Idle itself, but its branch is working: same pulse
                            // in the coordination accent so a delegating row is
                            // not read as finished. Takes precedence over the
                            // unread dot for the same reason a live turn does.
                            <span
                              className="relative flex h-2 w-2 shrink-0"
                              title={`${liveWorkers} worker çalışıyor`}
                            >
                              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--color-warning)] opacity-75" />
                              <span className="relative inline-flex h-2 w-2 rounded-full bg-[var(--color-warning)]" />
                            </span>
                          ) : (
                            s.unread && (
                              <span
                                className="h-2 w-2 shrink-0 rounded-full bg-[var(--color-accent)]"
                                title="Okunmadı"
                              />
                            )
                          )}
                          {/* Unsent composer text waiting in this session. Not a
                              run state, so it rides alongside the dot above rather
                              than replacing it. */}
                          {hasDraft && (
                            <span
                              className="flex shrink-0 text-[var(--color-accent)]"
                              title="Gönderilmemiş taslak mesaj var"
                            >
                              <PencilLine size={11} />
                            </span>
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
                          {displayedModel && (
                            <span
                              className="shrink-0 rounded bg-[var(--color-surface-2)] px-1 py-px font-mono text-[9px] text-[var(--color-text-dim)]"
                              title={`Model: ${displayedModel}`}
                            >
                              {modelDisplayName(displayedModel)}
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
                          <span className="ml-auto shrink-0 font-mono opacity-50" title="Oturum ID">
                            {s.id}
                          </span>
                        </span>
                      </span>
                    </button>
                  </div>
                )
              })}
            </div>
          ))}
        {!loading && groups.length === 0 && query.trim().length < 2 && (
          <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
            {chipsOff.length === 0 ? 'Oturum yok. + ile başlat.' : 'Seçili çiplerde oturum yok.'}
          </p>
        )}

        {/* TSK68 load-more: the list is paged; append the next page instead of
            fetching every session up front. Hidden while searching (message hits
            are their own section) or when the full list is already loaded. */}
        {shouldShowSessionsLoadMore({
          loading,
          hasMoreSessions: hasMoreSessions ?? false,
          canLoadMore: Boolean(onLoadMore),
          query,
          hasActiveChipFilters: chipsOff.length > 0,
          filteredSessionCount: orderedIds.length,
        }) &&
          onLoadMore && (
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
        {allSelectedArchived ? (
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
