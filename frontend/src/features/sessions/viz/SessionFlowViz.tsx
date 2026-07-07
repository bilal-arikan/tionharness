import { useEffect, useState } from 'react'
import { ChevronDown, ChevronRight, Loader2, Workflow } from 'lucide-react'
import { api } from '@/api'
import type { SessionDebugEvent } from '@/types'
import { ToolSankey } from './ToolSankey'
import { ConcurrencyTimeline } from './ConcurrencyTimeline'

// How many raw events to pull for the visualizations. Covers the whole span for
// typical sessions; very long ones are truncated to the newest window (noted).
const EVENT_LIMIT = 500

// SessionFlowViz is the foldable "workflow visualizations" section of the session
// debug view: a tool-execution Sankey (Agent → Tool → outcome) and a concurrency
// timeline (agent lanes over wall-clock). Both derive from the session's debug
// journal; events are fetched lazily the first time the section is opened.
export function SessionFlowViz({
  sessionId,
  agentNames,
  refreshKey,
}: {
  sessionId: string
  agentNames: Record<string, string>
  refreshKey?: number
}) {
  const [open, setOpen] = useState(
    () => localStorage.getItem('tionswarm.flowVizOpen') === '1',
  )
  const [events, setEvents] = useState<SessionDebugEvent[] | null>(null)
  const [loading, setLoading] = useState(false)

  const toggle = () =>
    setOpen((v) => {
      const next = !v
      localStorage.setItem('tionswarm.flowVizOpen', next ? '1' : '0')
      return next
    })

  // Lazy-fetch (and refresh) the raw events only while the section is open.
  useEffect(() => {
    if (!open) return
    let alive = true
    setLoading(true)
    api
      .sessionDebugEvents(sessionId, '', EVENT_LIMIT)
      .then((e) => alive && setEvents(e))
      .catch(() => alive && setEvents([]))
      .finally(() => alive && setLoading(false))
    return () => {
      alive = false
    }
  }, [open, sessionId, refreshKey])

  return (
    <section className="mt-3">
      <button
        onClick={toggle}
        className="flex w-full items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70 transition hover:opacity-100"
      >
        {open ? (
          <ChevronDown size={12} className="shrink-0" />
        ) : (
          <ChevronRight size={12} className="shrink-0" />
        )}
        <Workflow size={12} className="shrink-0" /> İş akışı görselleştirmeleri
      </button>

      {open && (
        <div className="mt-2">
          {loading && !events ? (
            <div className="flex items-center gap-1.5 py-2 text-[11px] text-[var(--color-text-dim)]">
              <Loader2 size={12} className="animate-spin" /> Yükleniyor…
            </div>
          ) : events && events.length > 0 ? (
            <div className="flex flex-col gap-3">
              <div>
                <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                  Araç yürütme akışı (Sankey)
                </div>
                <p className="mb-1 text-[10px] text-[var(--color-text-dim)]">
                  Ajandan araca çağrı akışı; bant kalınlığı çağrı sayısıyla orantılı,
                  varsa hata dalı ayrılır.
                </p>
                <ToolSankey events={events} agentNames={agentNames} />
              </div>

              <div>
                <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                  Eşzamanlılık zaman çizelgesi
                </div>
                <p className="mb-1 text-[10px] text-[var(--color-text-dim)]">
                  Ajan başına şeritte olayların zaman ekseni; şeritler arası çakışma
                  gerçek eşzamanlılığı gösterir.
                </p>
                <ConcurrencyTimeline events={events} agentNames={agentNames} />
              </div>

              {events.length >= EVENT_LIMIT && (
                <p className="text-[10px] text-[var(--color-text-dim)]">
                  Not: yalnız en son {EVENT_LIMIT} olay gösteriliyor.
                </p>
              )}
            </div>
          ) : (
            <p className="py-2 text-[11px] text-[var(--color-text-dim)]">
              Görselleştirilecek olay yok.
            </p>
          )}
        </div>
      )}
    </section>
  )
}
