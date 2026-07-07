import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { GitBranch, Activity, Copy, Table2 } from 'lucide-react'
import type { Agent, Message } from '@/types'
import { api } from '@/api'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { MessageList } from '@/features/chat/MessageList'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { relativeTime } from '@/shared/lib/time'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { useAsync } from '@/shared/hooks/useAsync'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { SelectionBar, SelectionBarButton, ListPane, PaneHeader } from '@/shared/components'
import { SidebarHeader, RefreshButton } from '@/shared/components/SidebarChrome'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { RevealButton } from '@/shared/components/RevealButton'
import { FILTERS, kindMeta, shortId, StatusPill } from './executionsShared'
import { SessionsOverview } from './SessionsOverview'

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

const POLL_MS = 5000
// Faster polling interval used when the selected execution is still running.
const RUNNING_POLL_MS = 2000

// ExecutionsPanel is the unified activity feed: a single list of every execution
// across chat / task / flow / schedule (each backed by a Session),
// with live status, plus a read-only transcript viewer for the selected one.
export function ExecutionsPanel({ agents, onError, onOpenFile, onOpenArtifact, onOpenFlowRun, focusId, onSelectExecution }: Props) {
  const [filter, setFilter] = useState('')
  // Bulk sessions overview overlay (searchable/sortable table of every session).
  const [overviewOpen, setOverviewOpen] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(focusId ?? null)
  const [messages, setMessages] = useState<Message[]>([])
  const [loading, setLoading] = useState(false)
  // Absolute on-disk folder of the selected execution's session (for the
  // open-folder + copy-path buttons in the detail header).
  const [sessPath, setSessPath] = useState('')
  const selectedRef = useRef<string | null>(null)
  selectedRef.current = selectedId

  // Multi-select (Ctrl/Cmd+Click, Shift-range). The feed is read-only, so the
  // one bulk action is copying the selected session ids (handy for cross-tooling).
  const sel = useMultiSelect()
  const { open: listOpen, toggle: toggleList, setOpen: setListOpen } = useCollapsibleList('tionswarm.executionsListOpen')
  // Landing on the screen with nothing selected: open the list drawer so a narrow
  // screen shows the pickable list instead of an empty detail pane. Runs once on
  // mount; on md+ the list is always visible so this is a no-op there.
  useEffect(() => {
    if (!selectedRef.current) setListOpen(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const bulkCopyIds = () => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    void copyToClipboard(ids.join('\n'))
    sel.clear()
  }

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

  // Initial + filter-change load, then poll so live "running" / status stays
  // fresh. Errors surface via onError.
  const {
    data: itemsData,
    error: itemsError,
    refresh: reloadItems,
  } = useAsync(() => api.listExecutions(filter || undefined), [filter], { pollMs: POLL_MS })
  const items = itemsData ?? []
  useEffect(() => {
    if (itemsError) onError(itemsError)
  }, [itemsError, onError])

  // Resolve the selected session's on-disk folder for the header path buttons.
  useEffect(() => {
    setSessPath('')
    if (!selectedId) return
    api
      .sessionPath(selectedId)
      .then((r) => {
        if (selectedRef.current === selectedId) setSessPath(r.path)
      })
      .catch(() => { /* best-effort; buttons fall back to a fetch on click */ })
  }, [selectedId])

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

  // Cross-window live sync: App.tsx's central SSE handler bumps the
  // 'executions' refresh signal on every chat / flow / schedule / spawn /
  // worker / task / session event in the active workspace. We re-pull the
  // list AND (if there's a selected execution) its transcript so the user
  // sees both the new row + the new assistant message. The central
  // dispatcher applies the workspace filter + 200ms debounce once; this
  // panel is just a consumer.
  const executionsTick = useRefreshTrigger('executions')
  useEffect(() => {
    reloadItems()
    const sid = selectedRef.current
    if (sid) {
      api
        .listMessages(sid)
        .then((m) => {
          if (selectedRef.current === sid) setMessages(m)
        })
        .catch(() => { /* best-effort */ })
    }
  }, [executionsTick, reloadItems])

  return (
    <div className="flex h-full min-h-0">
      {/* Master: the executions list — a full-height sibling column (like the chat
          sessions sidebar), so the title bar never spans over it. */}
      <ListPane
        open={listOpen}
        onToggle={toggleList}
        widthKey="tionswarm.executionsListWidth"
        defaultWidth={320}
        label="Yürütmeler"
        testId="executions-list-toggle"
        hideRail
      >
        <SidebarHeader title="Yürütmeler">
          <RefreshButton onClick={() => reloadItems()} />
        </SidebarHeader>

        {/* Full-width action opening the bulk sessions table (sized/placed like the
            chat sidebar's "Yeni Sohbet" button). */}
        <div className="px-3 pb-1 pt-1">
          <button
            onClick={() => setOverviewOpen(true)}
            title="Tüm oturumları tablo olarak gör"
            data-testid="sessions-overview-open"
            className="flex w-full items-center justify-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
          >
            <Table2 size={15} /> Oturumlar
          </button>
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
            const orderedIds = items.map((x) => x.sessionId)
            return (
              <button
                key={it.sessionId}
                onClick={(e) => {
                  if (sel.handleClick(e, it.sessionId, orderedIds, selectedId)) return
                  select(it.sessionId)
                }}
                className={`mb-0.5 flex w-full items-start gap-2 rounded-lg px-3 py-1.5 text-left text-sm transition ${
                  sel.isSelected(it.sessionId)
                    ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)] ring-1 ring-[var(--color-accent)]'
                    : isActive
                      ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
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
                    <span className="font-mono opacity-70">#{shortId(it.sessionId)}</span>
                    {it.agentName && <span className="truncate">· {it.agentName}</span>}
                    <span className="shrink-0">· {relativeTime(it.updatedAt)}</span>
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

        <SelectionBar
          count={sel.count}
          onClear={sel.clear}
          onSelectAll={items.length ? () => sel.selectAll(items.map((i) => i.sessionId)) : undefined}
        >
          <SelectionBarButton icon={<Copy size={13} />} onClick={bulkCopyIds}>
            Kimlikleri kopyala
          </SelectionBarButton>
        </SelectionBar>
      </ListPane>

      {/* Detail column: the standard title bar (PaneHeader) + the selected
          execution's transcript. The title bar sits ONLY above the content, to the
          right of the list — exactly like the chat header. */}
      <div className="flex h-full min-w-0 flex-1 flex-col">
        <PaneHeader
          title="Aktivite"
          subtitle={selected ? `· ${selected.title || kindMeta(selected.kind).label}` : undefined}
          listOpen={listOpen}
          onToggleList={toggleList}
          right={
            selected ? (
              <div className="flex items-center gap-1.5">
                {/* Path actions — same components + look as the chat header. */}
                <CopyPathButton
                  getPath={async () => sessPath || (await api.sessionPath(selected.sessionId)).path}
                  label="Yolu kopyala"
                  labelClassName="hidden"
                  title="Yolu kopyala"
                  onError={onError}
                />
                <RevealButton
                  onReveal={() => api.revealSession(selected.sessionId).catch((e) => onError((e as Error).message))}
                  label="Aç"
                  labelClassName="hidden sm:inline"
                  title={sessPath ? `Klasörü aç: ${sessPath}` : 'Klasörü aç'}
                />
                {selected.kind === 'flow' && selected.sourceId && onOpenFlowRun && (
                  <button
                    onClick={() => onOpenFlowRun(selected.sourceId!)}
                    title="Bu akışın koşularını Akışlar ekranında aç"
                    className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                  >
                    <GitBranch size={13} /> <span className="hidden sm:inline">Akış görünümü</span>
                  </button>
                )}
              </div>
            ) : undefined
          }
        />
        {selected ? (
          <>
            <header className="flex items-center gap-2 border-b border-[var(--color-border)] px-6 py-2">
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
                onClick={() => void copyToClipboard(selected.sessionId)}
                title={`Kimliği kopyala: ${selected.sessionId}`}
                className="flex items-center gap-1 rounded border border-[var(--color-border)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              >
                <Copy size={10} /> #{shortId(selected.sessionId)}
              </button>
              {selected.running && (
                <span className="ml-1 text-[11px] font-medium text-[var(--color-success)]">çalışıyor…</span>
              )}
            </header>
            <MessageList
              messages={messages}
              sessionId={selected.sessionId}
              pending={loading || selected.running}
              // A running execution (scheduled task / flow / spawn) is live: mark
              // it streaming so the last assistant bubble keeps the working
              // indicator through its tool steps until the turn completes.
              streaming={selected.running}
              pendingAgentId={selected.agentId}
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

      {/* Bulk sessions overview: a searchable/sortable table of every session,
          opened from the list header. Selecting a row jumps to its transcript. */}
      {overviewOpen && (
        <SessionsOverview
          items={items}
          agents={agents}
          onSelect={select}
          onClose={() => setOverviewOpen(false)}
        />
      )}
    </div>
  )
}
