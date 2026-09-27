import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ChevronDown, ChevronRight, Cpu, Square } from 'lucide-react'
import { api } from '@/api'
import type { ProcessEntry, ProcessKind, ProcessStatus } from '@/types'
import { clockTime, formatDurationMs, fullDateTime } from '@/shared/lib/time'
import { Badge, EmptyState, PaneHeader, type BadgeTone } from '@/shared/components'

interface Props {
  onError: (msg: string) => void
  // Open a session's transcript (App switches to the chat view). Absent when the
  // panel is rendered without a navigation host — the owner column then stays
  // plain text.
  onOpenSession?: (sessionId: string) => void
}

// How many entries the panel asks for. The backend keeps a bounded history, so
// this is a ceiling rather than a page size — there is no pagination here.
const LIST_LIMIT = 300

// The `process` SSE frame fires per state change, so starting a fan-out of
// workers emits a burst. Coalesce a burst into a single refetch.
const REFETCH_DEBOUNCE_MS = 300

const STATUS_LABEL: Record<ProcessStatus, string> = {
  running: 'Çalışıyor',
  succeeded: 'Başarılı',
  failed: 'Başarısız',
  killed: 'Durduruldu',
  timed_out: 'Zaman aşımı',
}

const STATUS_TONE: Record<ProcessStatus, BadgeTone> = {
  running: 'accent',
  succeeded: 'success',
  failed: 'danger',
  killed: 'warning',
  timed_out: 'warning',
}

const KIND_LABEL: Record<ProcessKind, string> = {
  shell: 'Kabuk',
  shell_background: 'Arka plan kabuk',
  code: 'Kod',
  provider: 'Sağlayıcı',
  mcp: 'MCP',
  hook: 'Hook',
  external: 'Harici araç',
}

const STATUS_OPTIONS: ProcessStatus[] = ['running', 'succeeded', 'failed', 'killed', 'timed_out']
const KIND_OPTIONS: ProcessKind[] = [
  'shell',
  'shell_background',
  'code',
  'provider',
  'mcp',
  'hook',
  'external',
]

// The "no session is linkable" verdict, shared so render-time fallbacks keep a
// stable identity.
const EMPTY_SESSIONS: ReadonlySet<string> = new Set()

// elapsedMs is how long the entry has been running, or how long it ran. `now` is
// passed in (rather than read here) so every row on one render shares an instant
// and the running rows re-tick together.
function elapsedMs(e: ProcessEntry, now: number): number {
  const end = e.endedAt && e.endedAt > 0 ? e.endedAt : now
  return Math.max(0, end - e.startedAt)
}

// ownerAgent is the agent side of the owner column: the name when the read side
// resolved one, the raw id otherwise. Empty for a process spawned outside any
// session (an MCP server for the workspace pool, an external tool update).
function ownerAgent(e: ProcessEntry): string {
  return e.owner.agentName || e.owner.agentId || ''
}

// ownerLabel flattens the owner column into one line, for the cell's tooltip and
// for the free-text filter.
function ownerLabel(e: ProcessEntry): string {
  const parts: string[] = []
  const agent = ownerAgent(e)
  if (agent) parts.push(agent)
  if (e.owner.sessionId) parts.push(e.owner.sessionId)
  if (e.owner.parentSessionId) parts.push(`üst: ${e.owner.parentSessionId}`)
  return parts.join(' · ')
}

// SessionRef renders one session id from the owner column. A process outlives
// the session that started it, so an id whose session is gone (deleted, or not
// resolvable right now) stays plain text instead of becoming a dead link.
function SessionRef({
  id,
  liveSessions,
  prefix,
  onOpen,
}: {
  id?: string
  liveSessions: ReadonlySet<string>
  prefix?: string
  onOpen?: (sessionId: string) => void
}) {
  if (!id) return null
  const text = prefix ? `${prefix} ${id}` : id
  if (!onOpen || !liveSessions.has(id))
    return <span className="block truncate font-mono">{text}</span>
  return (
    <button
      type="button"
      onClick={() => onOpen(id)}
      data-testid="process-session-link"
      title="Bu oturuma git"
      className="block max-w-full truncate text-left font-mono text-[var(--color-accent)] hover:underline"
    >
      {text}
    </button>
  )
}

// ProcessPanel is the Workspace window's "İşlemler" sub-page: every native
// process TionHarness started for this workspace — the live ones and the recent
// history — with status/kind/text filters, a stop action for the ones the
// backend can terminate, and per-row output/error detail.
//
// The list is SSE-driven: the shared feed's `process` frame carries no entry by
// design, so the panel refetches on it instead of merging (see api/system.ts).
export function ProcessPanel({ onError, onOpenSession }: Props) {
  const [entries, setEntries] = useState<ProcessEntry[]>([])
  const [loading, setLoading] = useState(true)
  // The last load failure. Kept in the panel (not only in a toast) so a failed
  // refresh can never look like "there are no processes".
  const [loadError, setLoadError] = useState('')
  const [status, setStatus] = useState<ProcessStatus | ''>('')
  const [kind, setKind] = useState<ProcessKind | ''>('')
  const [q, setQ] = useState('')
  const [expanded, setExpanded] = useState<string | null>(null)
  // Id of the row awaiting stop confirmation, then the one being stopped.
  const [confirmStop, setConfirmStop] = useState<string | null>(null)
  const [stopping, setStopping] = useState<string | null>(null)
  // Shared "now" for the duration column, re-read once a second while at least
  // one row is running.
  const [now, setNow] = useState(() => Date.now())
  // Which of the sessions named by the rows still exist. The ledger keeps a
  // process after its session is deleted, so the owner column can only link the
  // ids in here. Kept together with the id set it was resolved for, so a row set
  // whose answer has not arrived yet falls back to "nothing is linkable" during
  // render instead of reusing the previous list's verdict.
  const [resolved, setResolved] = useState<{ key: string; ids: ReadonlySet<string> }>(() => ({
    key: '',
    ids: EMPTY_SESSIONS,
  }))

  const load = useCallback(
    () =>
      api
        .listProcesses({
          status: status ? [status] : undefined,
          kind: kind ? [kind] : undefined,
          limit: LIST_LIMIT,
        })
        .then((data) => {
          setEntries(data)
          setLoadError('')
        })
        .catch((e) => {
          // Surface the failure in place AND to the app: the previously loaded
          // rows stay on screen, now explicitly marked as possibly stale.
          setLoadError((e as Error).message)
          onError((e as Error).message)
        })
        .finally(() => setLoading(false)),
    [status, kind, onError],
  )

  // Initial load + reload whenever a server-side filter changes.
  useEffect(() => {
    void load()
  }, [load])

  // Live: one refetch per burst of `process` frames, plus a resync when the
  // shared feed reopens after a drop (frames published while it was down are
  // gone for good).
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    const unsubProcesses = api.subscribeProcesses(() => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
      debounceRef.current = setTimeout(() => {
        debounceRef.current = null
        void load()
      }, REFETCH_DEBOUNCE_MS)
    })
    const unsubReconnect = api.subscribeReconnect(() => void load())
    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current)
        debounceRef.current = null
      }
      unsubProcesses()
      unsubReconnect()
    }
  }, [load])

  // The session ids the rows mention, as a stable key: a refetch that returns the
  // same owners must not re-run the existence lookup below.
  const sessionIdKey = useMemo(() => {
    const ids = new Set<string>()
    for (const e of entries) {
      if (e.owner.sessionId) ids.add(e.owner.sessionId)
      if (e.owner.parentSessionId) ids.add(e.owner.parentSessionId)
    }
    return [...ids].sort().join(',')
  }, [entries])

  const liveSessions = resolved.key === sessionIdKey ? resolved.ids : EMPTY_SESSIONS

  // Resolve which of those sessions are still there. A failure is reported, not
  // swallowed: the ids then render as plain text, which is also what a deleted
  // session gets, so a silent failure would look like "all sessions deleted".
  useEffect(() => {
    if (!sessionIdKey) return
    let cancelled = false
    api
      .getSessionsByIds(sessionIdKey.split(','))
      .then((items) => {
        if (!cancelled) setResolved({ key: sessionIdKey, ids: new Set(items.map((s) => s.id)) })
      })
      .catch((e) => {
        if (!cancelled) onError((e as Error).message)
      })
    return () => {
      cancelled = true
    }
  }, [sessionIdKey, onError])

  const hasRunning = useMemo(() => entries.some((e) => e.status === 'running'), [entries])

  // Tick the duration column only while something is actually running — a
  // history-only list has no moving number and should not wake the tab up.
  useEffect(() => {
    if (!hasRunning) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [hasRunning])

  // Free-text filter over command, label and owner, applied client-side (the
  // backend filters owners by EXACT id, which is the wrong shape for a search
  // box — "hangi ajan çalıştırdı" is answered by typing part of a name).
  const rows = useMemo(() => {
    const needle = q.trim().toLowerCase()
    if (!needle) return entries
    return entries.filter((e) =>
      [
        e.command,
        e.label,
        e.owner.agentName,
        e.owner.agentId,
        e.owner.sessionId,
        e.owner.parentSessionId,
      ].some((v) => (v ?? '').toLowerCase().includes(needle)),
    )
  }, [entries, q])

  const stop = useCallback(
    async (id: string) => {
      setConfirmStop(null)
      setStopping(id)
      try {
        const res = await api.stopProcess(id)
        // stopped=false is a 200 with a reason (already finished, or no stop
        // path): report it instead of pretending the click worked.
        if (!res.stopped) onError(res.reason ?? 'İşlem durdurulamadı.')
      } catch (e) {
        onError((e as Error).message)
      } finally {
        setStopping(null)
        await load()
      }
    },
    [load, onError],
  )

  const runningCount = useMemo(
    () => entries.filter((e) => e.status === 'running').length,
    [entries],
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PaneHeader
        title="İşlemler"
        right={
          <>
            <span className="text-xs text-[var(--color-text-dim)]">
              {`${rows.length} işlem · ${runningCount} çalışıyor`}
            </span>
            <button
              onClick={() => void load()}
              className="rounded border border-[var(--color-border)] px-2 py-1 text-xs hover:border-[var(--color-accent)]"
              title="İşlem listesini yenile"
            >
              Yenile
            </button>
          </>
        }
      />

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-4 py-2">
        <select
          value={status}
          onChange={(e) => setStatus(e.target.value as ProcessStatus | '')}
          className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
          title="Duruma göre filtrele"
          aria-label="Duruma göre filtrele"
        >
          <option value="">Durum: hepsi</option>
          {STATUS_OPTIONS.map((s) => (
            <option key={s} value={s}>
              {STATUS_LABEL[s]}
            </option>
          ))}
        </select>
        <select
          value={kind}
          onChange={(e) => setKind(e.target.value as ProcessKind | '')}
          className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
          title="Türe göre filtrele"
          aria-label="Türe göre filtrele"
        >
          <option value="">Tür: hepsi</option>
          {KIND_OPTIONS.map((k) => (
            <option key={k} value={k}>
              {KIND_LABEL[k]}
            </option>
          ))}
        </select>
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Ara (komut/etiket/ajan/oturum)…"
          className="min-w-40 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
          aria-label="Komut, etiket, ajan veya oturuma göre ara"
        />
        {/* Quick toggle: the "what is running right now" question, one click. */}
        <button
          onClick={() => setStatus((s) => (s === 'running' ? '' : 'running'))}
          aria-pressed={status === 'running'}
          className={`rounded px-2 py-1 text-xs transition ${
            status === 'running'
              ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
              : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
          }`}
        >
          Yalnız çalışanlar
        </button>
      </div>

      {loadError && (
        <div
          role="alert"
          className="border-b border-[var(--color-danger)] bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] px-4 py-2 text-xs text-[var(--color-danger)]"
        >
          İşlemler okunamadı: {loadError}
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        {loading ? (
          <p className="px-4 py-3 text-sm text-[var(--color-text-dim)]">Yükleniyor…</p>
        ) : rows.length === 0 ? (
          <EmptyState icon={Cpu} title="İşlem yok">
            {entries.length > 0
              ? 'Filtrelerle eşleşen işlem yok.'
              : 'Bu workspace için izlenen bir süreç bulunmuyor.'}
          </EmptyState>
        ) : (
          <table className="w-full table-fixed text-xs">
            <thead className="sticky top-0 bg-[var(--color-surface)] text-left text-[var(--color-text-dim)]">
              <tr className="border-b border-[var(--color-border)]">
                <th className="w-6 px-2 py-2" />
                <th className="w-24 px-2 py-2">Durum</th>
                <th className="w-28 px-2 py-2">Tür</th>
                <th className="px-2 py-2">Komut</th>
                <th className="w-20 px-2 py-2">PID</th>
                <th className="w-44 px-2 py-2">Sahip</th>
                <th className="w-32 px-2 py-2">Başlangıç</th>
                <th className="w-24 px-2 py-2">Süre</th>
                <th className="w-16 px-2 py-2">Çıkış</th>
                <th className="w-24 px-2 py-2" />
              </tr>
            </thead>
            <tbody>
              {rows.map((e) => {
                const open = expanded === e.id
                const terminal = e.status !== 'running'
                const owner = ownerLabel(e)
                const agent = ownerAgent(e)
                return [
                  <tr
                    key={e.id}
                    data-testid="process-row"
                    className="border-b border-[var(--color-border)]/40 align-top hover:bg-[var(--color-surface-2)]/40"
                  >
                    <td className="px-2 py-1.5">
                      <button
                        onClick={() => setExpanded(open ? null : e.id)}
                        aria-expanded={open}
                        aria-label={open ? 'Ayrıntıyı kapat' : 'Ayrıntıyı aç'}
                        className="text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
                      >
                        {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                      </button>
                    </td>
                    <td className="px-2 py-1.5" data-testid="process-status">
                      <Badge tone={STATUS_TONE[e.status]}>{STATUS_LABEL[e.status]}</Badge>
                    </td>
                    <td className="px-2 py-1.5 text-[var(--color-text-dim)]">
                      {KIND_LABEL[e.kind]}
                    </td>
                    <td className="px-2 py-1.5">
                      {e.label && (
                        <span className="mr-1 text-[var(--color-text-dim)]">{e.label}:</span>
                      )}
                      <span className="font-mono text-[var(--color-text)]" title={e.command}>
                        <span className="line-clamp-1 break-all">{e.command}</span>
                      </span>
                    </td>
                    <td className="px-2 py-1.5 font-mono text-[var(--color-text-dim)]">
                      {e.pid ? e.pid : '—'}
                    </td>
                    <td className="px-2 py-1.5 text-[var(--color-text-dim)]" title={owner}>
                      {owner ? (
                        <>
                          {agent && <span className="block truncate">{agent}</span>}
                          <SessionRef
                            id={e.owner.sessionId}
                            liveSessions={liveSessions}
                            onOpen={onOpenSession}
                          />
                          <SessionRef
                            id={e.owner.parentSessionId}
                            liveSessions={liveSessions}
                            prefix="üst:"
                            onOpen={onOpenSession}
                          />
                        </>
                      ) : (
                        '—'
                      )}
                    </td>
                    <td
                      className="px-2 py-1.5 text-[var(--color-text-dim)]"
                      title={fullDateTime(e.startedAt / 1000)}
                    >
                      {clockTime(e.startedAt / 1000)}
                    </td>
                    <td
                      className="px-2 py-1.5 text-[var(--color-text-dim)]"
                      data-testid="process-duration"
                    >
                      {formatDurationMs(elapsedMs(e, now))}
                    </td>
                    <td className="px-2 py-1.5 font-mono">
                      {terminal ? (
                        <span
                          className={
                            e.exitCode === 0
                              ? 'text-[var(--color-text-dim)]'
                              : 'text-[var(--color-danger)]'
                          }
                        >
                          {e.exitCode}
                        </span>
                      ) : (
                        <span className="text-[var(--color-text-dim)]">—</span>
                      )}
                    </td>
                    <td className="px-2 py-1.5 text-right">
                      {e.stoppable &&
                        (confirmStop === e.id ? (
                          <span className="flex items-center justify-end gap-1">
                            <button
                              onClick={() => void stop(e.id)}
                              data-testid="process-stop-confirm"
                              className="rounded bg-[var(--color-danger)] px-2 py-0.5 text-[10px] text-[var(--color-on-accent)]"
                            >
                              Emin misin?
                            </button>
                            <button
                              onClick={() => setConfirmStop(null)}
                              className="text-[10px] text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
                            >
                              Vazgeç
                            </button>
                          </span>
                        ) : (
                          <button
                            onClick={() => setConfirmStop(e.id)}
                            disabled={stopping === e.id}
                            data-testid="process-stop"
                            title="Bu süreci durdur"
                            className="inline-flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-0.5 text-[10px] text-[var(--color-text-dim)] hover:border-[var(--color-danger)] hover:text-[var(--color-danger)] disabled:opacity-50"
                          >
                            <Square size={10} />
                            {stopping === e.id ? 'Durduruluyor…' : 'Durdur'}
                          </button>
                        ))}
                    </td>
                  </tr>,
                  open && (
                    <tr key={`${e.id}-detail`} data-testid="process-detail">
                      <td colSpan={10} className="border-b border-[var(--color-border)] px-8 py-2">
                        <div className="space-y-2">
                          <pre className="whitespace-pre-wrap break-all font-mono text-[11px] text-[var(--color-text)]">
                            {e.command}
                          </pre>
                          {e.dir && (
                            <p className="font-mono text-[11px] text-[var(--color-text-dim)]">
                              {e.dir}
                            </p>
                          )}
                          {e.error && (
                            <p className="text-[11px] text-[var(--color-danger)]">{e.error}</p>
                          )}
                          {e.outputTail ? (
                            <pre className="max-h-64 overflow-auto rounded bg-[var(--color-surface-2)] p-2 font-mono text-[11px] text-[var(--color-text-dim)]">
                              {e.outputTail}
                            </pre>
                          ) : (
                            <p className="text-[11px] text-[var(--color-text-dim)]">
                              Bu süreç için kayıtlı çıktı yok.
                            </p>
                          )}
                        </div>
                      </td>
                    </tr>
                  ),
                ]
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
