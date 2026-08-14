import { useEffect, useRef, useState, useCallback, useMemo, type ReactNode } from 'react'
import { ArrowDown, Copy, Download } from 'lucide-react'
import { api } from '@/api'
import type { LogEntry } from '@/types'
import { groupConsecutive } from './logGroup'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { PaneHeader } from '@/shared/components'

interface Props {
  onError: (msg: string) => void
}

const LEVELS = ['', 'debug', 'info', 'warn', 'error'] as const
const LEVEL_LABEL: Record<string, string> = {
  '': 'Hepsi',
  debug: 'Debug',
  info: 'Info',
  warn: 'Warn',
  error: 'Error',
}

const LEVEL_COLOR: Record<string, string> = {
  ERROR: 'text-[var(--color-danger)]',
  WARN: 'text-[var(--color-warning)]',
  INFO: 'text-[var(--color-accent)]',
  DEBUG: 'text-[var(--color-text-dim)]',
}

// levelRank mirrors the backend ordering so SSE-appended entries respect the
// active minimum-level filter without a round trip.
const LEVEL_RANK: Record<string, number> = { DEBUG: 1, INFO: 2, WARN: 3, ERROR: 4 }

// Time-window presets for the since filter (minutes; '' = all retained).
const RANGES = [
  { key: '', label: 'Tümü' },
  { key: '15', label: '15 dk' },
  { key: '60', label: '1 saat' },
  { key: '1440', label: '24 saat' },
] as const

function clockTime(ms: number): string {
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

// Milliseconds fragment (".123"), shown only when the panel is wide enough.
function msPart(ms: number): string {
  return String(new Date(ms).getMilliseconds()).padStart(3, '0')
}

// Full HH:MM:SS.mmm, used for the hover tooltip so precision stays reachable
// even when the visible column drops milliseconds on a narrow panel.
function fmtTime(ms: number): string {
  return `${clockTime(ms)}.${msPart(ms)}`
}

// entryText flattens one entry to a copy/export-friendly single line.
function entryText(e: LogEntry): string {
  const parts = [new Date(e.time).toISOString(), e.level, e.message]
  if (e.component) parts.push(`component=${e.component}`)
  if (e.workspace) parts.push(`workspace=${e.workspace}`)
  if (e.agent) parts.push(`agent=${e.agent}`)
  if (e.session) parts.push(`session=${e.session}`)
  for (const [k, v] of Object.entries(e.attrs ?? {})) parts.push(`${k}=${v}`)
  return parts.join(' ')
}

// highlight wraps case-insensitive matches of q in <mark> so search hits are
// visible at a glance. Plain text when q is empty or absent from the string.
function highlight(text: string, q: string): ReactNode {
  if (!q) return text
  const lower = text.toLowerCase()
  const needle = q.toLowerCase()
  if (!lower.includes(needle)) return text
  const out: ReactNode[] = []
  let i = 0
  let hit = lower.indexOf(needle)
  let key = 0
  while (hit >= 0) {
    if (hit > i) out.push(text.slice(i, hit))
    out.push(
      <mark key={key++} className="rounded bg-[var(--color-warning)]/30 px-0.5 text-inherit">
        {text.slice(hit, hit + needle.length)}
      </mark>,
    )
    i = hit + needle.length
    hit = lower.indexOf(needle, i)
  }
  if (i < text.length) out.push(text.slice(i))
  return out
}

// LogsPanel shows the application + all-workspace log stream from the backend
// ring buffer: live SSE tail, level/component/time filtering, debounced text
// search with match highlighting, per-line copy, JSON export and optional
// collapsing of consecutive identical entries into a single counted row.
export function LogsPanel({ onError }: Props) {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [level, setLevel] = useState<string>('')
  const [q, setQ] = useState('')
  // Debounced copy of q: the API reload + highlight work run on this, so fast
  // typing doesn't fire a request per keystroke.
  const [qDebounced, setQDebounced] = useState('')
  const [component, setComponent] = useState('')
  const [range, setRange] = useState<string>('')
  const [follow, setFollow] = useState(true)
  const [group, setGroup] = useState(true)
  // Count of live entries that arrived (matching the filters) while follow was
  // off — surfaced as a "N yeni kayıt" refresh chip instead of moving the view.
  const [pending, setPending] = useState(0)
  // Absolute path of the on-disk log file (for copy / reveal in Explorer).
  const [logPath, setLogPath] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)
  // Whether the viewport is pinned to the bottom. While true, new lines
  // auto-scroll into view; once the user scrolls up this turns false and we
  // stop yanking the view down (a "jump to bottom" button appears instead).
  const [atBottom, setAtBottom] = useState(true)
  const atBottomRef = useRef(true)

  // Resolve the on-disk log file path once for the copy/open-folder actions.
  useEffect(() => {
    api
      .logsPath()
      .then((r) => setLogPath(r.path))
      .catch(() => setLogPath(''))
  }, [])

  // Debounce the search box (the input itself stays instant).
  useEffect(() => {
    const id = setTimeout(() => setQDebounced(q.trim()), 300)
    return () => clearTimeout(id)
  }, [q])

  const sinceMs = useCallback((): number | undefined => {
    const mins = Number(range)
    return mins > 0 ? Date.now() - mins * 60_000 : undefined
  }, [range])

  const load = useCallback(async () => {
    try {
      const data = await api.getLogs({
        limit: 1000,
        level: level || undefined,
        q: qDebounced || undefined,
        component: component || undefined,
        since: sinceMs(),
      })
      setLogs(data)
      setPending(0)
    } catch (e) {
      onError((e as Error).message)
    }
  }, [level, qDebounced, component, sinceMs, onError])

  // Initial + reactive load when filters change.
  useEffect(() => {
    void load()
  }, [load])

  // Client-side twin of the server filters, applied to live SSE entries.
  const matchesFilters = useCallback(
    (e: LogEntry): boolean => {
      if (level && (LEVEL_RANK[e.level] ?? 0) < (LEVEL_RANK[level.toUpperCase()] ?? 0)) return false
      if (component && e.component !== component) return false
      const since = sinceMs()
      if (since && e.time < since) return false
      if (qDebounced) {
        const needle = qDebounced.toLowerCase()
        const hay = entryText(e).toLowerCase()
        if (!hay.includes(needle)) return false
      }
      return true
    },
    [level, component, sinceMs, qDebounced],
  )

  // Live tail over the shared SSE feed: matching entries append directly while
  // following; while paused they only bump the "new records" chip. A slow 30s
  // reconciliation poll (follow only) backfills anything missed across an SSE
  // reconnect.
  useEffect(() => {
    const unsub = api.subscribeLogs((entry) => {
      if (!matchesFilters(entry)) return
      if (follow) {
        setLogs((prev) => {
          if (prev.length > 0 && entry.seq <= prev[prev.length - 1].seq) return prev
          const next = [...prev, entry]
          return next.length > 1000 ? next.slice(next.length - 1000) : next
        })
      } else {
        setPending((n) => n + 1)
      }
    })
    return unsub
  }, [follow, matchesFilters])

  useEffect(() => {
    if (!follow) return
    const id = setInterval(() => {
      void load()
    }, 30_000)
    return () => clearInterval(id)
  }, [follow, load])

  // Collapse runs of identical adjacent entries when grouping is enabled.
  const rows = useMemo(
    () =>
      group
        ? groupConsecutive(logs)
        : logs.map((e) => ({ entry: e, count: 1, firstTime: e.time, lastTime: e.time })),
    [logs, group],
  )

  // Distinct components present in the current result set (plus the active
  // selection, so a filter that empties the list stays visible/undoable).
  const components = useMemo(() => {
    const set = new Set<string>()
    for (const e of logs) if (e.component) set.add(e.component)
    if (component) set.add(component)
    return [...set].sort()
  }, [logs, component])

  // Track whether the viewport is pinned to the bottom. A small threshold
  // tolerates sub-pixel rounding and lets near-bottom still count as "bottom".
  const onScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    const pinned = el.scrollHeight - el.scrollTop - el.clientHeight < 24
    atBottomRef.current = pinned
    setAtBottom(pinned)
  }, [])

  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
    atBottomRef.current = true
    setAtBottom(true)
  }, [])

  // Auto-scroll to the newest line only when following AND the user has not
  // scrolled up. If they scrolled away from the bottom, leave the view alone
  // so reading older lines isn't interrupted by incoming logs.
  useEffect(() => {
    if (follow && atBottomRef.current && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [rows, follow])

  const copyLine = useCallback((e: LogEntry) => {
    void navigator.clipboard?.writeText(entryText(e)).catch(() => {})
  }, [])

  // Download the currently filtered entries as a JSON file.
  const exportLogs = useCallback(() => {
    const blob = new Blob([JSON.stringify(logs, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `tionswarm-logs-${new Date().toISOString().replace(/[:.]/g, '-')}.json`
    a.click()
    URL.revokeObjectURL(url)
  }, [logs])

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PaneHeader
        title="Loglar"
        right={
          <>
            <span className="text-xs text-[var(--color-text-dim)]">
              {group && rows.length !== logs.length
                ? `${rows.length} satır · ${logs.length} kayıt`
                : `${logs.length} kayıt`}
            </span>
            <CopyPathButton path={logPath} title="Log dosyası yolunu kopyala" />
            <button
              onClick={exportLogs}
              className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1 text-xs hover:border-[var(--color-accent)]"
              title="Filtrelenmiş logları JSON olarak indir"
            >
              <Download size={12} />
              <span className="hidden sm:inline">İndir</span>
            </button>
            <button
              onClick={() => void load()}
              className="rounded border border-[var(--color-border)] px-2 py-1 text-xs hover:border-[var(--color-accent)]"
              title="Logları yenile"
            >
              Yenile
            </button>
          </>
        }
      />
      {/* Filters (count / copy-path / open-folder moved to the title bar above). */}
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-4 py-2">
        <div className="flex items-center gap-1">
          {LEVELS.map((l) => (
            <button
              key={l || 'all'}
              onClick={() => setLevel(l)}
              className={`rounded px-2 py-1 text-xs transition ${
                level === l
                  ? 'bg-[var(--color-accent)] text-white'
                  : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
              }`}
            >
              {LEVEL_LABEL[l]}
            </button>
          ))}
        </div>
        <select
          value={component}
          onChange={(e) => setComponent(e.target.value)}
          className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
          title="Bileşene göre filtrele"
        >
          <option value="">Bileşen: hepsi</option>
          {components.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
        <select
          value={range}
          onChange={(e) => setRange(e.target.value)}
          className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
          title="Zaman aralığına göre filtrele"
        >
          {RANGES.map((r) => (
            <option key={r.key} value={r.key}>
              {r.label}
            </option>
          ))}
        </select>
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Ara (mesaj/alan)…"
          className="min-w-40 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
        />
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          <input type="checkbox" checked={group} onChange={(e) => setGroup(e.target.checked)} />
          Grupla
        </label>
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          <input type="checkbox" checked={follow} onChange={(e) => setFollow(e.target.checked)} />
          Canlı
        </label>
        {!follow && pending > 0 && (
          <button
            onClick={() => void load()}
            className="rounded-full border border-[var(--color-accent)] px-2 py-0.5 text-xs text-[var(--color-accent)] hover:bg-[var(--color-accent)]/10"
            title="Takip kapalıyken gelen yeni kayıtları yükle"
          >
            {pending} yeni kayıt — Yenile
          </button>
        )}
      </div>

      {/* Log lines */}
      <div className="relative flex min-h-0 flex-1 flex-col">
        <div
          ref={scrollRef}
          onScroll={onScroll}
          className="@container flex-1 overflow-y-auto p-3 font-mono text-xs leading-relaxed"
        >
          {rows.length === 0 && <p className="text-[var(--color-text-dim)]">Kayıt yok.</p>}
          {rows.map((g) => {
            const e = g.entry
            return (
              <div
                key={e.seq}
                className="group flex gap-2 border-b border-[var(--color-border)]/30 py-0.5"
                // Skip layout/paint for offscreen rows — cheap virtualization that
                // keeps a 1000-row list responsive without a windowing library.
                style={{ contentVisibility: 'auto', containIntrinsicSize: 'auto 22px' }}
              >
                <span
                  className="shrink-0 text-[var(--color-text-dim)]"
                  title={
                    g.count > 1
                      ? `${clockTime(g.firstTime)} → ${clockTime(g.lastTime)}`
                      : fmtTime(e.time)
                  }
                >
                  {clockTime(e.time)}
                  {/* Milliseconds only on a genuinely wide panel (@2xl ≈ 672px) — on
                    narrow/medium widths they crowd the row and add little (the full
                    time incl. ms is always in the row's title tooltip). */}
                  <span className="hidden @2xl:inline">.{msPart(e.time)}</span>
                </span>
                {/* Level: single-letter (I/W/E/D) when narrow, full label when wide. */}
                <span
                  className={`w-3 shrink-0 font-semibold @sm:w-12 ${LEVEL_COLOR[e.level] ?? ''}`}
                  title={e.level}
                >
                  <span className="@sm:hidden">{e.level.charAt(0)}</span>
                  <span className="hidden @sm:inline">{e.level}</span>
                </span>
                {e.component && (
                  <button
                    onClick={() => setComponent(e.component!)}
                    className="hidden shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 text-[var(--color-text-dim)] hover:text-[var(--color-accent)] @sm:inline"
                    title={`Bileşene göre filtrele: ${e.component}`}
                  >
                    {e.component}
                  </button>
                )}
                {g.count > 1 && (
                  <span
                    className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 font-semibold text-[var(--color-accent)]"
                    title={`${g.count} kez tekrarlandı (${clockTime(g.firstTime)} → ${clockTime(g.lastTime)})`}
                  >
                    ×{g.count}
                  </span>
                )}
                <span className="min-w-0 flex-1 break-words">
                  <span className="text-[var(--color-text)]">
                    {highlight(e.message, qDebounced)}
                  </span>
                  {e.session && (
                    <span className="ml-2 text-[var(--color-text-dim)]">
                      session=
                      <span className="text-[var(--color-accent)]">
                        {highlight(e.session, qDebounced)}
                      </span>
                    </span>
                  )}
                  {e.agent && (
                    <span className="ml-2 text-[var(--color-text-dim)]">
                      agent=
                      <span className="text-[var(--color-accent)]">
                        {highlight(e.agent, qDebounced)}
                      </span>
                    </span>
                  )}
                  {e.workspace && (
                    <span className="ml-2 text-[var(--color-text-dim)]">
                      workspace=
                      <span className="text-[var(--color-accent)]">
                        {highlight(e.workspace, qDebounced)}
                      </span>
                    </span>
                  )}
                  {e.attrs &&
                    Object.entries(e.attrs).map(([k, v]) => (
                      <span key={k} className="ml-2 text-[var(--color-text-dim)]">
                        {k}=
                        <span className="text-[var(--color-accent)]">
                          {highlight(v, qDebounced)}
                        </span>
                      </span>
                    ))}
                </span>
                <button
                  onClick={() => copyLine(e)}
                  className="invisible shrink-0 self-start text-[var(--color-text-dim)] hover:text-[var(--color-accent)] group-hover:visible"
                  title="Satırı kopyala"
                >
                  <Copy size={12} />
                </button>
              </div>
            )
          })}
        </div>
        {/* Jump-to-bottom: shown when following but the user scrolled up, so new
          lines no longer drag the view down. Click re-pins to the bottom. */}
        {follow && !atBottom && (
          <button
            onClick={scrollToBottom}
            title="En alta in"
            className="absolute bottom-3 right-4 flex items-center gap-1 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-1.5 text-xs text-[var(--color-text)] shadow-md transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
          >
            <ArrowDown size={12} />
            En alta in
          </button>
        )}
      </div>
    </div>
  )
}
