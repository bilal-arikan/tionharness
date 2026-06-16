import { useEffect, useRef, useState, useCallback } from 'react'
import { api } from '../api'
import type { LogEntry } from '../types'

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
  ERROR: 'text-red-400',
  WARN: 'text-amber-400',
  INFO: 'text-sky-400',
  DEBUG: 'text-[var(--color-text-dim)]',
}

function fmtTime(ms: number): string {
  const d = new Date(ms)
  const p = (n: number, w = 2) => String(n).padStart(w, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}.${p(d.getMilliseconds(), 3)}`
}

// LogsPanel shows the application + all-workspace log stream from the backend
// ring buffer, with live follow, level filtering and text search.
export function LogsPanel({ onError }: Props) {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [level, setLevel] = useState<string>('')
  const [q, setQ] = useState('')
  const [follow, setFollow] = useState(true)
  const scrollRef = useRef<HTMLDivElement>(null)

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

  // Auto-scroll to the newest line when following.
  useEffect(() => {
    if (follow && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [logs, follow])

  return (
    <div className="flex h-full flex-col">
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
          <input type="checkbox" checked={follow} onChange={(e) => setFollow(e.target.checked)} />
          Canlı
        </label>
        <button
          onClick={() => void load()}
          className="rounded border border-[var(--color-border)] px-2 py-1 text-xs hover:border-[var(--color-accent)]"
        >
          Yenile
        </button>
        <span className="text-xs text-[var(--color-text-dim)]">{logs.length} kayıt</span>
      </div>

      {/* Log lines */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto p-3 font-mono text-xs leading-relaxed">
        {logs.length === 0 && (
          <p className="text-[var(--color-text-dim)]">Kayıt yok.</p>
        )}
        {logs.map((e) => (
          <div key={e.seq} className="flex gap-2 border-b border-[var(--color-border)]/30 py-0.5">
            <span className="shrink-0 text-[var(--color-text-dim)]">{fmtTime(e.time)}</span>
            <span className={`w-12 shrink-0 font-semibold ${LEVEL_COLOR[e.level] ?? ''}`}>{e.level}</span>
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
        ))}
      </div>
    </div>
  )
}
