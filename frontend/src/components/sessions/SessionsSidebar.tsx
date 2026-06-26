import { useEffect, useMemo, useRef, useState } from 'react'
import { Settings, Pencil, Sparkles, ClipboardCopy, FolderOpen, Trash2, Search, X, MessageSquareText, Plus, RefreshCw, Archive, ArchiveRestore, type LucideIcon } from 'lucide-react'
import type { Agent, Session, SearchHit } from '../../types'
import { api } from '../../api'
import { AgentAvatar } from '../agents/AgentAvatar'
import { relativeTime, bucketOf, BUCKET_LABELS, BUCKET_ORDER, type Bucket } from '../../lib/time'
import { useOutsideClick } from '../../hooks/useOutsideClick'

interface Props {
  sessions: Session[]
  agents: Agent[]
  activeSessionId: string | null
  // Sessions whose turns are currently being generated (live), shown with a
  // pulsing indicator so in-progress conversations are visible from the list.
  // A set because several turns can stream concurrently (detached server-side).
  streamingSessionIds?: ReadonlySet<string>
  newDisabled: boolean
  // messageId is set when the user clicks a message-content search result, so the
  // transcript can scroll to that exact turn.
  onSelectSession: (id: string, messageId?: string) => void
  onNewSession: () => void
  // Re-fetch the session list from the server (manual refresh button).
  onRefresh: () => void
  onRenameSession: (id: string, title: string) => void
  onGenerateTitle: (id: string) => void
  onCopyPath: (id: string) => void
  onRevealFolder: (id: string) => void
  onDeleteSession: (id: string) => void
  // Archive (true) or restore (false) a session — drives the Active/Archived filter.
  onSetArchived: (id: string, archived: boolean) => void
}

// SessionsSidebar is the chat column: a flat, time-bucketed list of every
// session (newest first), with per-row unread dots and a settings menu
// (rename, AI title, copy path, open folder, delete).
export function SessionsSidebar({
  sessions,
  agents,
  activeSessionId,
  streamingSessionIds,
  newDisabled,
  onSelectSession,
  onNewSession,
  onRefresh,
  onRenameSession,
  onGenerateTitle,
  onCopyPath,
  onRevealFolder,
  onDeleteSession,
  onSetArchived,
}: Props) {
  const [menuId, setMenuId] = useState<string | null>(null)
  // Active vs Archived view. Archiving a session moves it out of the default
  // (active) list into the Archived filter — it is never deleted.
  const [showArchived, setShowArchived] = useState(false)
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
    const saved = Number(localStorage.getItem('swarmgo.sidebarWidth'))
    return saved >= MIN && saved <= MAX ? saved : 264
  })
  const drag = useRef<{ startX: number; startW: number } | null>(null)
  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!drag.current) return
      setWidth(Math.min(MAX, Math.max(MIN, drag.current.startW + (e.clientX - drag.current.startX))))
    }
    const onUp = () => {
      if (!drag.current) return
      drag.current = null
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
      localStorage.setItem('swarmgo.sidebarWidth', String(width))
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


  // How many sessions are archived (drives the Archived filter's count badge).
  const archivedCount = useMemo(() => sessions.filter((s) => s.state === 'archived').length, [sessions])

  // Group the (already newest-first) sessions into recency buckets, preserving
  // order. The Active/Archived filter narrows by state first, then a title search.
  const groups = useMemo(() => {
    const q = query.trim().toLowerCase()
    const map = new Map<Bucket, Session[]>()
    for (const s of sessions) {
      const isArchived = s.state === 'archived'
      if (showArchived !== isArchived) continue
      if (q && !(s.title || 'Yeni sohbet').toLowerCase().includes(q)) continue
      const b = bucketOf(s.updatedAt)
      const arr = map.get(b) ?? []
      arr.push(s)
      map.set(b, arr)
    }
    return BUCKET_ORDER.filter((b) => map.has(b)).map((b) => ({ bucket: b, items: map.get(b)! }))
  }, [sessions, query, showArchived])

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

      {/* Title search */}
      <div className="relative px-3 pb-2 pt-1">
        <Search size={13} className="pointer-events-none absolute left-5 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]" />
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

      {/* Active / Archived filter. The Archived pill carries a count so archived
          work is discoverable without cluttering the default list. */}
      <div className="flex gap-1 px-3 pb-2">
        <button
          onClick={() => setShowArchived(false)}
          className={`flex-1 rounded-md px-2 py-1 text-xs font-medium transition ${
            !showArchived
              ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
              : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
          }`}
        >
          Aktif
        </button>
        <button
          onClick={() => setShowArchived(true)}
          className={`flex flex-1 items-center justify-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition ${
            showArchived
              ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
              : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
          }`}
        >
          <Archive size={12} /> Arşiv{archivedCount > 0 ? ` (${archivedCount})` : ''}
        </button>
      </div>

      <div className="flex-1 overflow-y-auto px-2 pb-2">
        {groups.map(({ bucket, items }) => (
          <div key={bucket} className="mb-1">
            <div className="px-3 pt-3 pb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
              {BUCKET_LABELS[bucket]}
            </div>
            {items.map((s) => {
              const owner = agents.find((a) => a.id === s.agentId)
              const isActive = activeSessionId === s.id
              const isStreaming = streamingSessionIds?.has(s.id) ?? false
              return (
                <div
                  key={s.id}
                  className={`group relative mb-0.5 flex w-full items-center rounded-lg pr-1 text-sm transition ${
                    isActive
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
                      onClick={() => onSelectSession(s.id)}
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
                            <span className="relative flex h-2 w-2 shrink-0" title="Yanıt üretiliyor">
                              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--color-success)] opacity-75" />
                              <span className="relative inline-flex h-2 w-2 rounded-full bg-[var(--color-success)]" />
                            </span>
                          ) : (
                            s.unread && (
                              <span className="h-2 w-2 shrink-0 rounded-full bg-[var(--color-accent)]" title="Okunmadı" />
                            )
                          )}
                          <span className={`min-w-0 flex-1 truncate ${s.unread || isStreaming ? 'font-semibold text-[var(--color-text)]' : ''}`}>
                            {s.title || 'Yeni sohbet'}
                          </span>
                          <span className="shrink-0 font-mono text-[9px] opacity-50" title="Oturum ID">
                            {s.id}
                          </span>
                        </span>
                        {isStreaming ? (
                          <span className="truncate text-[10px] font-medium text-[var(--color-success)]">yazıyor…</span>
                        ) : (
                          <span className="truncate text-[10px] opacity-60">
                            {relativeTime(s.updatedAt)} · {s.messageCount} mesaj
                          </span>
                        )}
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
                      <MenuItem icon={Pencil} label="Başlığı düzenle" onClick={() => startRename(s)} />
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
                        icon={FolderOpen}
                        label="Klasörü aç"
                        onClick={() => {
                          onRevealFolder(s.id)
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
                          if (confirm(`"${s.title || 'Bu oturum'}" silinsin mi?`)) onDeleteSession(s.id)
                        }}
                      />
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        ))}
        {groups.length === 0 && query.trim().length < 2 && (
          <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
            {showArchived ? 'Arşivlenmiş oturum yok.' : 'Oturum yok. + ile başlat.'}
          </p>
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
                <span className="line-clamp-2 text-xs text-[var(--color-text-dim)]">{h.snippet}</span>
              </button>
            ))}
          </div>
        )}
      </div>

      <div
        onMouseDown={startDrag}
        title="Genişliği ayarla"
        className="absolute right-0 top-0 h-full w-1 cursor-col-resize bg-transparent transition hover:bg-[var(--color-accent)]"
      />
    </aside>
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
