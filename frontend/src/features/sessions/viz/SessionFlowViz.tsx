import { useEffect, useState } from 'react'
import { ChevronDown, ChevronRight, Loader2, Workflow } from 'lucide-react'
import { api } from '@/api'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import type { Hook, SessionDebugEvent } from '@/types'
import { ToolSankey } from './ToolSankey'
import { ConcurrencyTimeline } from './ConcurrencyTimeline'
import { SelfHealingEvents } from './SelfHealingEvents'
import { PromptCacheEvents } from './PromptCacheEvents'
import { ThinkingShareChart } from './ThinkingShareChart'
import { HookActivity } from './HookActivity'
import { useTranslation } from 'react-i18next'

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
  const { t } = useTranslation('sessions')
  const [open, setOpen] = useState(() => localStorage.getItem('tionharness.flowVizOpen') === '1')
  const [events, setEvents] = useState<SessionDebugEvent[] | null>(null)
  const [hooks, setHooks] = useState<Hook[]>([])
  // An open section fetches on mount, so it starts in the loading state.
  const [loading, setLoading] = useState(open)

  const toggle = () =>
    setOpen((v) => {
      const next = !v
      localStorage.setItem('tionharness.flowVizOpen', next ? '1' : '0')
      return next
    })

  // Lazy-fetch (and refresh) the raw events only while the section is open;
  // each trigger re-arms the spinner before the fetch lands via callbacks.
  useKeyedReset(`${open}|${sessionId}|${refreshKey}`, () => {
    if (open) setLoading(true)
  })
  useEffect(() => {
    if (!open) return
    let alive = true
    api
      .sessionDebugEvents(sessionId, '', EVENT_LIMIT)
      .then((e) => alive && setEvents(e))
      .catch(() => alive && setEvents([]))
      .finally(() => alive && setLoading(false))
    // Hooks (workspace-scoped) attribute type=hook events to rtk/sqz in HookActivity.
    api
      .listHooks()
      .then((h) => alive && setHooks(h))
      .catch(() => alive && setHooks([]))
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
        <Workflow size={12} className="shrink-0" /> {t('visualization.title')}
      </button>

      {open && (
        <div className="mt-2">
          {loading && !events ? (
            <div className="flex items-center gap-1.5 py-2 text-[11px] text-[var(--color-text-dim)]">
              <Loader2 size={12} className="animate-spin" /> {t('common.loading')}
            </div>
          ) : events && events.length > 0 ? (
            <div className="flex flex-col gap-3">
              <div>
                <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                  {t('visualization.sankey.title')}
                </div>
                <p className="mb-1 text-[10px] text-[var(--color-text-dim)]">
                  {t('visualization.sankey.help')}
                </p>
                <ToolSankey events={events} agentNames={agentNames} />
              </div>

              <div>
                <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                  {t('visualization.timeline.title')}
                </div>
                <p className="mb-1 text-[10px] text-[var(--color-text-dim)]">
                  {t('visualization.timeline.help')}
                </p>
                <ConcurrencyTimeline events={events} agentNames={agentNames} />
              </div>

              <div>
                <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                  {t('visualization.selfHealing.title')}
                </div>
                <p className="mb-1 text-[10px] text-[var(--color-text-dim)]">
                  {t('visualization.selfHealing.help')}
                </p>
                <SelfHealingEvents events={events} />
              </div>

              <div>
                <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                  {t('visualization.promptCache.title')}
                </div>
                <p className="mb-1 text-[10px] text-[var(--color-text-dim)]">
                  {t('visualization.promptCache.help')}
                </p>
                <PromptCacheEvents events={events} />
              </div>

              <div>
                <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                  {t('visualization.thinking.title')}
                </div>
                <p className="mb-1 text-[10px] text-[var(--color-text-dim)]">
                  {t('visualization.thinking.description')}
                </p>
                <ThinkingShareChart events={events} />
              </div>

              <div>
                <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                  {t('visualization.hooks.title')}
                </div>
                <p className="mb-1 text-[10px] text-[var(--color-text-dim)]">
                  {t('visualization.hooks.description')}
                </p>
                <HookActivity events={events} hooks={hooks} />
              </div>

              {events.length >= EVENT_LIMIT && (
                <p className="text-[10px] text-[var(--color-text-dim)]">
                  {t('visualization.truncated', { count: EVENT_LIMIT })}
                </p>
              )}
            </div>
          ) : (
            <p className="py-2 text-[11px] text-[var(--color-text-dim)]">
              {t('visualization.empty')}
            </p>
          )}
        </div>
      )}
    </section>
  )
}
