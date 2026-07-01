import { useEffect, useRef, useState, useCallback, useMemo } from 'react'
import { ArrowDown } from 'lucide-react'
import { api } from '../../api'
import type { LogEntry } from '../../types'
import { groupConsecutive } from '../../lib/logGroup'
import { CopyPathButton } from '../CopyPathButton'
import { RevealButton } from '../RevealButton'

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

function fmtTime(ms: number): string {
  const d = new Date(ms)
  const p = (n: number, w = 2) => String(n).padStart(w, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}.${p(d.getMilliseconds(), 3)}`
}

function clockTime(ms: number): string {
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

// LogsPanel shows the application + all-workspace log stream from the backend
// ring buffer, with live follow, level filtering, text search, and optional
// collapsing of consecutive identical entries into a single counted row.
export function LogsPanel({ onError }: Props) {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [level, setLevel] = useState<string>('')
  const [q, setQ] = useState('')
  const [follow, setFollow] = useState(true)
  const [group, setGroup] = useState(true)
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
    api.logsPath().then((r) => setLogPath(r.path)).catch(() => setLogPath(''))
  }, [])

  const load = useCallback(async () => {
    try {
      const data = await api.getLogs({ limit: 1000, level: level || undefined, q: q || undefined })
      setLogs(data)
    } catch (e) {
      onError((e as Error).message)
    }
  }, [level, q, onError])

  // Initial + reactive load when filters change.
  useEffect(() => { void load() }, [load])

  // Live polling while "follow" is on.
  useEffect(() => {
    if (!follow) return
    const id = setInterval(() => { void load() }, 2500)
    return () => clearInterval(id)
  }, [follow, load])

  // Collapse runs of identical adjacent entries when grouping is enabled.
  const rows = useMemo(
    () => (group ? groupConsecutive(logs) : logs.map((e) => ({ entry: e, count: 1, firstTime: e.time, lastTime: e.time }))),
    [logs, group],
  )

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

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Controls */}
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
        <button
          onClick={() => void load()}
          className="rounded border border-[var(--color-border)] px-2 py-1 text-xs hover:border-[var(--color-accent)]"
        >
          Yenile
        </button>
        {/* On-disk log file: copy its path or open its folder in Explorer. */}
        <CopyPathButton path={logPath} title="Log dosyası yolunu kopyala" />
        <RevealButton
          onReveal={() => api.revealLogs().catch((e) => onError((e as Error).message))}
          title="Log klasörünü aç"
        />
        <span className="text-xs text-[var(--color-text-dim)]">
          {group && rows.length !== logs.length ? `${rows.length} satır · ${logs.length} kayıt` : `${logs.length} kayıt`}
        </span>
      </div>

      {/* Log lines */}
      <div className="relative flex min-h-0 flex-1 flex-col">
      <div ref={scrollRef} onScroll={onScroll} className="flex-1 overflow-y-auto p-3 font-mono text-xs leading-relaxed">
        {rows.length === 0 && (
          <p className="text-[var(--color-text-dim)]">Kayıt yok.</p>
        )}
        {rows.map((g) => {
          const e = g.entry
          return (
            <div key={e.seq} className="flex gap-2 border-b border-[var(--color-border)]/30 py-0.5">
              <span
                className="shrink-0 text-[var(--color-text-dim)]"
                title={g.count > 1 ? `${clockTime(g.firstTime)} → ${clockTime(g.lastTime)}` : undefined}
              >
                {fmtTime(e.time)}
              </span>
              <span className={`w-12 shrink-0 font-semibold ${LEVEL_COLOR[e.level] ?? ''}`}>{e.level}</span>
              {g.count > 1 && (
                <span
                  className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 font-semibold text-[var(--color-accent)]"
                  title={`${g.count} kez tekrarlandı (${clockTime(g.firstTime)} → ${clockTime(g.lastTime)})`}
                >
                  ×{g.count}
                </span>
              )}
              <span className="min-w-0 flex-1 break-words">
                <span className="text-[var(--color-text)]">{e.message}</span>
                {e.attrs &&
                  Object.entries(e.attrs).map(([k, v]) => (
                    <span key={k} className="ml-2 text-[var(--color-text-dim)]">
                      {k}=<span className="text-[var(--color-accent)]">{v}</span>
                    </span>
                  ))}
              </span>
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
