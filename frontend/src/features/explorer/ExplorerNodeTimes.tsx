import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Eye, PenLine } from 'lucide-react'
import type { ViewGraphTimes } from '@/types'
import { relativeAge } from './explorerAttention'

interface Props {
  times: ViewGraphTimes | undefined
  // Display name of an agent id (the map's agent node label); falls back to the id.
  agentLabel: (id: string) => string
}

// ExplorerNodeTimes is the side panel's recency line for the selected node: who
// last read it (an agent, through get_view / read_artifact / use_skill /
// note_expand) and when it was last edited — the same stamps the time window
// filters on. Renders nothing for a node without any.
export function ExplorerNodeTimes({ times, agentLabel }: Props) {
  const { t } = useTranslation('explorer')
  const [nowMs, setNowMs] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNowMs(Date.now()), 30_000)
    return () => window.clearInterval(timer)
  }, [])
  if (!times) return null
  const ago = (at: number | undefined) => {
    const age = relativeAge(at, nowMs)
    return age ? t(`selection.times.age.${age.unit}`, { count: age.count }) : null
  }
  const read = ago(times.read)
  const edited = ago(times.updated)
  const created = edited ? null : ago(times.created)
  if (!read && !edited && !created) return null
  return (
    <div className="flex flex-col gap-0.5 text-[11px] text-[var(--color-text-dim)]">
      {read && (
        <span className="flex items-center gap-1.5 truncate">
          <Eye size={12} className="shrink-0" />
          {times.readBy
            ? t('selection.times.readBy', { agent: agentLabel(times.readBy), age: read })
            : t('selection.times.readByUnknown', { age: read })}
        </span>
      )}
      {(edited || created) && (
        <span className="flex items-center gap-1.5 truncate">
          <PenLine size={12} className="shrink-0" />
          {edited
            ? t('selection.times.edited', { age: edited })
            : t('selection.times.created', { age: created })}
        </span>
      )}
    </div>
  )
}
